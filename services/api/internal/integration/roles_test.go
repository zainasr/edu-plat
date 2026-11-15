package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
)

// addStaff gives a tenant a second person with one built-in role and returns their session token.
// The role assignment is data, so it is inserted the way an admin screen will later do it.
func addStaff(t *testing.T, f *fixture, tenantID, slug, role string) string {
	t.Helper()
	ctx := context.Background()
	mobile := newMobile(t)
	var userID, membershipID string
	if err := f.admin.QueryRow(ctx, `INSERT INTO users (mobile, full_name) VALUES ($1, $2) RETURNING id`, mobile, "Staff "+role).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := f.admin.QueryRow(ctx, `INSERT INTO memberships (tenant_id, user_id) VALUES ($1, $2) RETURNING id`, tenantID, userID).Scan(&membershipID); err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if _, err := f.admin.Exec(ctx, `INSERT INTO role_assignments (tenant_id, membership_id, role) VALUES ($1, $2, $3)`, tenantID, membershipID, role); err != nil {
		t.Fatalf("assign role %s: %v", role, err)
	}
	return signIn(t, f, mobile, slug)
}

// REQ-IAM-005, REQ-IAM-002: the built-in roles hold exactly the permissions the spec gives them.
// Every endpoint is called with a body that would be refused (400/404) if the permission passed, so
// anything other than 403 means the role may call it. A role is "allowed" or "forbidden" per endpoint.
func TestREQ_IAM_005_BuiltInRolesHoldTheirPermissions(t *testing.T) {
	f := newFixture(t)
	slug, ownerMobile, tenantID := newTenant(t, f, "roles")
	tokens := map[string]string{
		"owner":        signIn(t, f, ownerMobile, slug),
		"accountant":   addStaff(t, f, tenantID, slug, "accountant"),
		"receptionist": addStaff(t, f, tenantID, slug, "receptionist"),
	}
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees), f.iam)

	const id = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	line := `{"fee_type_id":"` + id + `","effective_from":"2026-04-01","amount":{"amount":1,"currency":"PKR"},"frequency":"monthly"}`
	endpoints := []struct {
		name, method, path, body string
		// who may call it
		owner, accountant, receptionist bool
	}{
		{"fee.settings: create fee type", http.MethodPost, "/api/v1/fee-types", `{"code":"lab_fee","name":"Lab","kind":"lab"}`, true, false, false},
		{"fee.view: list fee types", http.MethodGet, "/api/v1/fee-types", "", true, true, true},
		{"fee.plan.manage: set plan line", http.MethodPost, "/api/v1/fee-plans/" + id + "/lines", line, true, true, false},
		{"fee.view: get plan", http.MethodGet, "/api/v1/fee-plans/" + id, "", true, true, true},
		{"fee.generate: generate challans", http.MethodPost, "/api/v1/challans/generate", `{}`, true, true, false},
		{"fee.release: release challans", http.MethodPost, "/api/v1/challans/release", `{"challan_ids":[]}`, true, true, false},
		{"fee.generate: void a draft", http.MethodPost, "/api/v1/challans/" + id + "/void", `{"reason":"x"}`, true, true, false},
		{"fee.view: list challans", http.MethodGet, "/api/v1/challans", "", true, true, true},
		{"fee.view: get challan", http.MethodGet, "/api/v1/challans/" + id, "", true, true, true},
		{"ledger.view: list accounts", http.MethodGet, "/api/v1/accounts", "", true, true, false},
		{"student.view: list students", http.MethodGet, "/api/v1/students", "", true, false, true},
		{"student.create: enrol a student", http.MethodPost, "/api/v1/students", `{}`, true, false, false},
		{"org.manage: create class", http.MethodPost, "/api/v1/classes", `{}`, true, false, false},
		{"user.view: list roles", http.MethodGet, "/api/v1/roles", "", true, false, false},
		{"user.view: list staff", http.MethodGet, "/api/v1/staff", "", true, false, false},
		{"user.view: get staff", http.MethodGet, "/api/v1/staff/" + id, "", true, false, false},
		{"user.manage: add staff", http.MethodPost, "/api/v1/staff", `{}`, true, false, false},
		{"role.manage: give a role", http.MethodPost, "/api/v1/staff/" + id + "/roles", `{"role":"accountant"}`, true, false, false},
		{"role.manage: take a role", http.MethodPost, "/api/v1/staff/" + id + "/roles/accountant/revoke", "", true, false, false},
		{"user.manage: deactivate", http.MethodPost, "/api/v1/staff/" + id + "/deactivate", "", true, false, false},
		{"user.manage: reactivate", http.MethodPost, "/api/v1/staff/" + id + "/reactivate", "", true, false, false},
	}
	for _, e := range endpoints {
		for role, want := range map[string]bool{"owner": e.owner, "accountant": e.accountant, "receptionist": e.receptionist} {
			t.Run(role+" "+e.name, func(t *testing.T) {
				rec := do(t, h, e.method, e.path, tokens[role], e.body)
				forbidden := rec.Code == http.StatusForbidden
				if rec.Code == http.StatusUnauthorized {
					t.Fatalf("the session was refused: %s", rec.Body)
				}
				if want && forbidden {
					t.Fatalf("%s must be allowed to call this, got 403: %s", role, rec.Body)
				}
				if !want && !forbidden {
					t.Fatalf("%s must NOT be allowed to call this, got %d: %s", role, rec.Code, rec.Body)
				}
			})
		}
	}
}
