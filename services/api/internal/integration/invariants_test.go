package integration

// Invariants: properties that must hold for EVERY slice, present and future. They query the live
// database catalog and the OpenAPI file, so they check what is true, not what a slice remembered
// to test. When a new table, endpoint or grant breaks a rule, one of these fails by itself.
//
// Run just these with:  make evaluate   (or: go test -run TestINV_ ./internal/integration/)

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/platform/idempotency"
)

// tenantTablesSQL lists every table in the public schema that has a tenant_id column.
const tenantTablesSQL = `
	SELECT c.relname
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE n.nspname = 'public' AND c.relkind = 'r'
	  AND EXISTS (SELECT 1 FROM information_schema.columns k
	              WHERE k.table_schema = 'public' AND k.table_name = c.relname AND k.column_name = 'tenant_id')
	ORDER BY c.relname`

// names runs a query that returns one text column and collects it.
func names(t *testing.T, f *fixture, sql string, args ...any) []string {
	t.Helper()
	rows, err := f.admin.Query(context.Background(), sql, args...)
	if err != nil {
		t.Fatalf("query: %v\n%s", err, sql)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// requireNone fails with every offender listed, so one run shows the whole problem.
func requireNone(t *testing.T, offenders []string, rule string) {
	t.Helper()
	if len(offenders) > 0 {
		t.Fatalf("%s\noffenders (%d):\n  - %s", rule, len(offenders), strings.Join(offenders, "\n  - "))
	}
}

// ADR-0002, REQ-ORG-001: every table with a tenant_id has row-level security switched on and a
// tenant_isolation policy that fails closed (NULLIF turns an unset tenant into NULL).
func TestINV_EveryTenantTableHasRLSAndAClosedPolicy(t *testing.T) {
	f := newFixture(t)
	tables := names(t, f, tenantTablesSQL)
	if len(tables) < 10 {
		t.Fatalf("found only %d tenant tables; the catalog query is probably wrong", len(tables))
	}
	var bad []string
	for _, table := range tables {
		var rls bool
		if err := f.admin.QueryRow(context.Background(),
			`SELECT relrowsecurity FROM pg_class WHERE oid = ('public.' || $1)::regclass`, table).Scan(&rls); err != nil {
			t.Fatal(err)
		}
		if !rls {
			bad = append(bad, table+": row level security is off")
			continue
		}
		var qual, check string
		err := f.admin.QueryRow(context.Background(),
			`SELECT coalesce(qual, ''), coalesce(with_check, '') FROM pg_policies
			 WHERE schemaname = 'public' AND tablename = $1 AND policyname = 'tenant_isolation'`, table).Scan(&qual, &check)
		if err != nil {
			bad = append(bad, table+": no tenant_isolation policy")
			continue
		}
		if !strings.Contains(strings.ToLower(qual), "nullif") || !strings.Contains(strings.ToLower(check), "nullif") {
			bad = append(bad, table+": policy does not use NULLIF, so an unset tenant is an error, not an empty result")
		}
	}
	requireNone(t, bad, "every tenant table needs ENABLE ROW LEVEL SECURITY and the tenant_isolation policy (see migration 00003)")
}

// sql-migrations rule: every tenant table has an index that starts with tenant_id.
func TestINV_EveryTenantTableHasATenantLeadingIndex(t *testing.T) {
	f := newFixture(t)
	bad := names(t, f, `
		SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r'
		  AND EXISTS (SELECT 1 FROM information_schema.columns k WHERE k.table_schema='public' AND k.table_name=c.relname AND k.column_name='tenant_id')
		  AND NOT EXISTS (
		      SELECT 1 FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[0]
		      WHERE i.indrelid = c.oid AND a.attname = 'tenant_id')
		ORDER BY 1`)
	requireNone(t, bad, "every tenant table needs an index whose first column is tenant_id")
}

// Composite tenant foreign keys: a row of one tenant can never point at a row of another.
func TestINV_ForeignKeysBetweenTenantTablesIncludeTheTenant(t *testing.T) {
	f := newFixture(t)
	bad := names(t, f, `
		SELECT conrelid::regclass::text || ' ' || conname || ': ' || pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE contype = 'f' AND connamespace = 'public'::regnamespace
		  AND EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = conrelid  AND a.attname = 'tenant_id' AND NOT a.attisdropped)
		  AND EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = confrelid AND a.attname = 'tenant_id' AND NOT a.attisdropped)
		  AND pg_get_constraintdef(oid) NOT LIKE 'FOREIGN KEY (tenant_id,%'
		ORDER BY 1`)
	requireNone(t, bad, "a foreign key between two tenant tables must start with tenant_id so it cannot cross tenants")
}

// DEC-011: business records are never hard-deleted. The API role has no DELETE and no TRUNCATE
// (TRUNCATE ignores row-level security) on any table.
func TestINV_TheAPIRoleCannotDeleteOrTruncateAnything(t *testing.T) {
	f := newFixture(t)
	bad := names(t, f, `
		SELECT c.relname || ' (' || p.priv || ')'
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		CROSS JOIN (VALUES ('DELETE'), ('TRUNCATE')) AS p(priv)
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND has_table_privilege('app_rt', c.oid, p.priv)
		ORDER BY 1`)
	requireNone(t, bad, "app_rt must hold no DELETE or TRUNCATE privilege (DEC-011): archive or reverse instead")
}

// The API role is bound by every rule above: not a superuser, no BYPASSRLS, owns no table.
func TestINV_TheAPIRoleIsNotPrivileged(t *testing.T) {
	f := newFixture(t)
	var super, bypass bool
	if err := f.admin.QueryRow(context.Background(),
		`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'app_rt'`).Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if super || bypass {
		t.Fatalf("app_rt: superuser=%v bypassrls=%v; either one switches RLS off", super, bypass)
	}
	owned := names(t, f, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tableowner = 'app_rt'`)
	requireNone(t, owned, "a table owner bypasses RLS; app_rt must own nothing")
}

// ADR-0004: money is whole paisa in a bigint, never numeric, float or int4.
func TestINV_MoneyColumnsAreBigint(t *testing.T) {
	f := newFixture(t)
	bad := names(t, f, `
		SELECT table_name || '.' || column_name || ' is ' || data_type
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND column_name ~ '(^|_)(amount|debit|credit|price|total|balance|paisa)$'
		  AND data_type <> 'bigint'
		ORDER BY 1`)
	requireNone(t, bad, "money columns must be bigint minor units (ADR-0004)")
}

// DEC-011: tables that hold history carry the append-only trigger.
func TestINV_HistoryTablesAreAppendOnly(t *testing.T) {
	f := newFixture(t)
	var bad []string
	for _, table := range []string{"audit_log", "journal_entries", "journal_lines", "fee_plan_lines", "student_fee_overrides", "challan_lines", "payment_allocations"} {
		var n int
		if err := f.admin.QueryRow(context.Background(), `
			SELECT count(*) FROM pg_trigger
			WHERE tgrelid = ('public.' || $1)::regclass AND NOT tgisinternal AND tgname LIKE '%append_only'`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			bad = append(bad, fmt.Sprintf("%s has %d append-only triggers, want 1", table, n))
		}
	}
	requireNone(t, bad, "history tables must refuse UPDATE and DELETE for every role")
}

// operation is one OpenAPI operation read from api/openapi/openapi.yaml.
type operation struct {
	ID         string
	Method     string
	Path       string
	Public     bool
	Idempotent bool // declares the Idempotency-Key header
}

var (
	pathLine   = regexp.MustCompile(`^  (/[^\s:]+):\s*$`)
	methodLine = regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	opIDLine   = regexp.MustCompile(`^      operationId:\s*(\w+)\s*$`)
	paramInURL = regexp.MustCompile(`\{[^}]+\}`)
)

// readOperations parses the paths section of the OpenAPI file with plain line matching, so this
// test adds no YAML dependency. An operation is public when it declares `security: []`.
func readOperations(t *testing.T) []operation {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "api", "openapi", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	var ops []operation
	var path string
	cur := -1
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "components:") {
			break
		}
		if m := pathLine.FindStringSubmatch(line); m != nil {
			path, cur = m[1], -1
			continue
		}
		if m := methodLine.FindStringSubmatch(line); m != nil && path != "" {
			ops = append(ops, operation{Method: strings.ToUpper(m[1]), Path: path})
			cur = len(ops) - 1
			continue
		}
		if cur < 0 {
			continue
		}
		if m := opIDLine.FindStringSubmatch(line); m != nil {
			ops[cur].ID = m[1]
		}
		if strings.TrimSpace(line) == "security: []" && strings.HasPrefix(line, "      security:") {
			ops[cur].Public = true
		}
		if strings.Contains(line, "#/components/parameters/IdempotencyKey") {
			ops[cur].Idempotent = true
		}
	}
	// Self-check: the parser must find exactly as many operations as the file declares. A silent
	// parser failure would make the authorisation test pass while testing nothing.
	top, _, _ := strings.Cut(string(raw), "\ncomponents:")
	if want := strings.Count(top, "operationId:"); len(ops) != want {
		t.Fatalf("parsed %d operations but the contract declares %d operationIds; fix readOperations", len(ops), want)
	}
	return ops
}

// The public surface is deliberate and tiny. Adding to it is a security decision a human reviews:
// edit this list in the same change, and say why in the pull request.
var publicOperations = []string{"getHealth", "requestOtp", "verifyOtp"}

// REQ-API-002, API conventions §3: no endpoint is reachable without a token, and none runs without
// its named permission. The test reads every operation from the contract and calls it three ways.
func TestINV_EveryOperationRequiresAuthAndAPermission(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	slug, mobile, tenantID := newTenant(t, f, "inv-authz")
	session := signIn(t, f, mobile, slug)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees), f.iam)
	// The session stays valid but holds no permission once every role is removed.
	if _, err := f.admin.Exec(ctx, `DELETE FROM role_assignments WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}

	ops := readOperations(t)
	if len(ops) == 0 {
		t.Fatal("read no operations from openapi.yaml")
	}

	var public []string
	for _, op := range ops {
		if op.ID == "" {
			t.Fatalf("%s %s has no operationId", op.Method, op.Path)
		}
		if op.Public {
			public = append(public, op.ID)
		}
	}
	slices.Sort(public)
	want := slices.Clone(publicOperations)
	slices.Sort(want)
	if !slices.Equal(public, want) {
		t.Fatalf("the public operations in openapi.yaml are %v, but this test allows only %v.\n"+
			"A new public endpoint is a security decision: get a human to approve it, then update publicOperations.", public, want)
	}

	const id = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	for _, op := range ops {
		if op.Public {
			continue
		}
		url := paramInURL.ReplaceAllString(op.Path, id)
		body := ""
		if op.Method == http.MethodPost || op.Method == http.MethodPut || op.Method == http.MethodPatch {
			body = `{}`
		}
		t.Run(op.ID+" anonymous is 401", func(t *testing.T) {
			if rec := do(t, h, op.Method, url, "", body); rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: status %d, want 401. Does the handler call iam.Require first? %s", op.Method, url, rec.Code, rec.Body)
			}
		})
		t.Run(op.ID+" without a permission is 403", func(t *testing.T) {
			if rec := do(t, h, op.Method, url, session, body); rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s: status %d, want 403. Does the handler require a named permission? %s", op.Method, url, rec.Code, rec.Body)
			}
		})
	}
}

// REQ-API-010: the operations that declare the Idempotency-Key header in the contract are exactly the
// routes the middleware honours. Add a header to an operation without listing it (or the reverse) and
// the client is promised something the server does not do.
func TestINV_IdempotencyOperationsMatchTheContract(t *testing.T) {
	var declared []string
	for _, op := range readOperations(t) {
		if op.Idempotent {
			declared = append(declared, op.Method+" "+op.Path)
		}
	}
	if len(declared) == 0 {
		t.Fatal("no operation declares the Idempotency-Key header; the parser may be broken")
	}
	var bad []string
	for _, d := range declared {
		if !slices.Contains(idempotency.Operations, d) {
			bad = append(bad, d+": declares the header but the middleware does not honour it")
		}
	}
	for _, o := range idempotency.Operations {
		if !slices.Contains(declared, o) {
			bad = append(bad, o+": honoured by the middleware but the contract does not declare the header")
		}
		if !strings.HasPrefix(o, "POST ") {
			bad = append(bad, o+": only POST can be idempotent here")
		}
	}
	requireNone(t, bad, "Idempotency-Key must match between the contract and the middleware")
}
