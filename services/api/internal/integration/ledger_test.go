package integration

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/ledger"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// billing is a balanced entry: a fee is billed (receivable up, income up).
func billing(amount money.Money, on string, t *testing.T) ledger.Entry {
	t.Helper()
	return ledger.Entry{
		OccurredOn: mustDate(t, on),
		Memo:       "Tuition, Ali Khan",
		SourceType: "challan.released",
		Lines: []ledger.Line{
			{AccountCode: ledger.AccountFeeReceivable, Debit: amount},
			{AccountCode: ledger.AccountFeeIncome, Credit: amount},
		},
	}
}

// tryPost posts inside a fresh tenant transaction and returns the result and the error.
func tryPost(f *fixture, p iam.Principal, e ledger.Entry) (ledger.PostedEntry, error) {
	var out ledger.PostedEntry
	err := db.WithTenant(context.Background(), f.app, p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = f.ledger.Post(context.Background(), tx, p, e)
		return err
	})
	return out, err
}

func mustPost(t *testing.T, f *fixture, p iam.Principal, e ledger.Entry) ledger.PostedEntry {
	t.Helper()
	out, err := tryPost(f, p, e)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return out
}

func ledgerPrincipal(t *testing.T, f *fixture, label string) (iam.Principal, string) {
	t.Helper()
	slug, mobile, tenantID := newTenant(t, f, label)
	return principalFor(t, f, slug, mobile), tenantID
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// ADR-0004: a posted entry is stored with its lines, and numbers count 1, 2, 3 with no gaps.
func TestADR_0004_PostedEntriesGetConsecutiveNumbers(t *testing.T) {
	f := newFixture(t)
	p, _ := ledgerPrincipal(t, f, "led-seq")

	for want := int64(1); want <= 3; want++ {
		got := mustPost(t, f, p, billing(500000, "2026-11-07", t))
		if got.EntryNo != want {
			t.Fatalf("entry number = %d, want %d", got.EntryNo, want)
		}
		if len(got.Lines) != 2 {
			t.Fatalf("lines = %d, want 2", len(got.Lines))
		}
	}
}

// ADR-0004 / BR-FEE-002: a post that rolls back gives its number back.
func TestADR_0004_AFailedPostGivesItsNumberBack(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, _ := ledgerPrincipal(t, f, "led-gap")

	boom := errors.New("the business step after the post failed")
	err := db.WithTenant(ctx, f.app, p.TenantID, func(tx pgx.Tx) error {
		if _, err := f.ledger.Post(ctx, tx, p, billing(100, "2026-11-07", t)); err != nil {
			return err
		}
		return boom // roll the whole transaction back after the post succeeded
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want the business error back, got %v", err)
	}

	got := mustPost(t, f, p, billing(100, "2026-11-07", t))
	if got.EntryNo != 1 {
		t.Fatalf("entry number = %d after a rollback, want 1 (no gap)", got.EntryNo)
	}
}

// ADR-0004: twenty simultaneous posts get twenty distinct consecutive numbers and the books balance.
func TestADR_0004_ConcurrentPostsAreGaplessAndBalanced(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, _ := ledgerPrincipal(t, f, "led-conc")

	const n = 20
	numbers := make(chan int64, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		e := billing(250000, "2026-11-07", t) // built on the test goroutine
		wg.Go(func() {
			got, err := tryPost(f, p, e)
			numbers <- got.EntryNo
			errs <- err
		})
	}
	wg.Wait()
	close(numbers)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent post failed: %v", err)
		}
	}
	var got []int64
	for no := range numbers {
		got = append(got, no)
	}
	slices.Sort(got)
	for i, no := range got {
		if no != int64(i+1) {
			t.Fatalf("numbers are not 1..%d with no gaps: %v", n, got)
		}
	}

	tb, err := f.ledger.TrialBalance(ctx, p, mustDate(t, "2026-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	if !tb.Balanced || tb.TotalDebit != money.Money(n*250000) {
		t.Fatalf("trial balance: balanced=%v debit=%v, want balanced %v", tb.Balanced, tb.TotalDebit, money.Money(n*250000))
	}
}

// ADR-0004, the guarantee: even raw SQL cannot commit an unbalanced entry. The check runs at commit
// in the database, so a bug in Go cannot unbalance the books.
func TestADR_0004_DatabaseRefusesAnUnbalancedEntryAtCommit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, tenantID := ledgerPrincipal(t, f, "led-db")
	mustPost(t, f, p, billing(100, "2026-11-07", t)) // creates the system accounts

	var cash, income string
	err := f.admin.QueryRow(ctx, `SELECT
		(SELECT id::text FROM accounts WHERE tenant_id = $1 AND code = '1000'),
		(SELECT id::text FROM accounts WHERE tenant_id = $1 AND code = '4000')`, tenantID).Scan(&cash, &income)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		lines [][3]any // account id, debit, credit
	}{
		{name: "debit and credit differ", lines: [][3]any{{cash, 100, 0}, {income, 0, 40}}},
		{name: "only one line", lines: [][3]any{{cash, 100, 0}}},
		{name: "no lines at all", lines: nil},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.WithTenant(ctx, f.app, tenantID, func(tx pgx.Tx) error {
				var entryID string
				if err := tx.QueryRow(ctx, `
					INSERT INTO journal_entries (tenant_id, entry_no, occurred_on, memo, source_type)
					VALUES ($1, $2, '2026-11-07', 'raw', 'test') RETURNING id::text`,
					tenantID, 1000+i).Scan(&entryID); err != nil {
					return err
				}
				for n, l := range tt.lines {
					if _, err := tx.Exec(ctx, `
						INSERT INTO journal_lines (tenant_id, entry_id, line_no, account_id, debit, credit)
						VALUES ($1, $2, $3, $4, $5, $6)`, tenantID, entryID, n+1, l[0], l[1], l[2]); err != nil {
						return err
					}
				}
				return nil
			})
			if pgCode(err) != "23514" {
				t.Fatalf("want a check violation (23514) at commit, got %v", err)
			}
		})
	}
}

// A single line is a debit or a credit, never both, never negative, never zero.
func TestADR_0004_DatabaseRefusesAMalformedLine(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, tenantID := ledgerPrincipal(t, f, "led-line")
	posted := mustPost(t, f, p, billing(100, "2026-11-07", t))

	var cash string
	if err := f.admin.QueryRow(ctx, `SELECT id::text FROM accounts WHERE tenant_id = $1 AND code = '1000'`, tenantID).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name          string
		debit, credit int64
	}{
		{"both sides", 5, 5},
		{"neither side", 0, 0},
		{"negative debit", -5, 0},
		{"negative credit", 0, -5},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := db.WithTenant(ctx, f.app, tenantID, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `
					INSERT INTO journal_lines (tenant_id, entry_id, line_no, account_id, debit, credit)
					VALUES ($1, $2, 9, $3, $4, $5)`, tenantID, posted.ID, cash, c.debit, c.credit)
				return err
			})
			if pgCode(err) != "23514" {
				t.Fatalf("want a check violation, got %v", err)
			}
		})
	}
}

// DEC-011: journal rows are never updated or deleted, by the API role or by the owner.
func TestDEC_011_JournalRowsCannotBeChanged(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, tenantID := ledgerPrincipal(t, f, "led-immut")
	posted := mustPost(t, f, p, billing(100, "2026-11-07", t))

	statements := []string{
		`UPDATE journal_entries SET memo = 'edited' WHERE id = $1`,
		`DELETE FROM journal_entries WHERE id = $1`,
		`UPDATE journal_lines SET debit = 999 WHERE entry_id = $1`,
		`DELETE FROM journal_lines WHERE entry_id = $1`,
	}
	for _, sql := range statements {
		t.Run("API role: "+sql, func(t *testing.T) {
			err := db.WithTenant(ctx, f.app, tenantID, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, sql, posted.ID)
				return err
			})
			if pgCode(err) != "42501" { // insufficient_privilege: the role has no UPDATE or DELETE grant
				t.Fatalf("want permission denied (42501), got %v", err)
			}
		})
		t.Run("owner: "+sql, func(t *testing.T) {
			_, err := f.admin.Exec(ctx, sql, posted.ID)
			if err == nil || !strings.Contains(err.Error(), "append-only") {
				t.Fatalf("want the append-only trigger, got %v", err)
			}
		})
	}
}

// A line that names no account is refused, nothing is stored, and no number is used up.
func TestADR_0004_UnknownAccountIsRefusedAndNothingIsStored(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, tenantID := ledgerPrincipal(t, f, "led-unk")

	e := billing(100, "2026-11-07", t)
	e.Lines[1].AccountCode = "9999"
	_, err := tryPost(f, p, e)
	if got := errCode(t, err); got != "ledger.unknown_account" {
		t.Fatalf("code = %s, want ledger.unknown_account", got)
	}
	var n int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("entries stored after a refusal: %v, %d", err, n)
	}
	if got := mustPost(t, f, p, billing(100, "2026-11-07", t)); got.EntryNo != 1 {
		t.Fatalf("entry number = %d, want 1", got.EntryNo)
	}
}

// A validation failure in Go never reaches the database.
func TestADR_0004_AnUnbalancedEntryIsRefusedBeforeTheDatabase(t *testing.T) {
	f := newFixture(t)
	p, _ := ledgerPrincipal(t, f, "led-unbal")
	e := billing(100, "2026-11-07", t)
	e.Lines[1].Credit = 99
	_, err := tryPost(f, p, e)
	if got := errCode(t, err); got != "ledger.unbalanced" {
		t.Fatalf("code = %s, want ledger.unbalanced", got)
	}
}

// ADR-0014: the built-in chart is created on first use, and matches the Go constants.
func TestADR_0014_SystemAccountsMatchTheConstants(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, _ := ledgerPrincipal(t, f, "led-chart")

	if list, err := f.ledger.ListAccounts(ctx, p); err != nil || len(list) != 0 {
		t.Fatalf("before the first post a school has no accounts: %v, %d", err, len(list))
	}
	mustPost(t, f, p, billing(100, "2026-11-07", t))

	accounts, err := f.ledger.ListAccounts(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	types := map[string]string{}
	for _, a := range accounts {
		codes = append(codes, a.Code)
		types[a.Code] = a.Type
		if !a.System {
			t.Fatalf("account %s should be a system account", a.Code)
		}
	}
	want := slices.Clone(ledger.SystemAccountCodes)
	slices.Sort(want)
	if !slices.Equal(codes, want) {
		t.Fatalf("accounts in the database %v differ from the Go constants %v", codes, want)
	}
	// Security deposits are a liability, never income (BR-FEE-004).
	if types[ledger.AccountSecurityDeposits] != "liability" || types[ledger.AccountFeeIncome] != "income" ||
		types[ledger.AccountFeeReceivable] != "asset" {
		t.Fatalf("account types are wrong: %v", types)
	}
}

// ADR-0004: a reversal mirrors the original, links to it, and leaves the original untouched. After
// both, every account's debit and credit totals are equal.
func TestADR_0004_ReversalNetsToZeroAndLeavesTheOriginal(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, _ := ledgerPrincipal(t, f, "led-rev")
	orig := mustPost(t, f, p, billing(500000, "2026-11-07", t))

	var rev ledger.PostedEntry
	err := db.WithTenant(ctx, f.app, p.TenantID, func(tx pgx.Tx) error {
		var err error
		rev, err = f.ledger.Reverse(ctx, tx, p, orig.ID, "billed the wrong child", mustDate(t, "2026-11-08"))
		return err
	})
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if rev.ReversesID != orig.ID || rev.EntryNo != orig.EntryNo+1 {
		t.Fatalf("reversal link/number wrong: %+v", rev)
	}
	if !strings.Contains(rev.Memo, "Reversal of #1: billed the wrong child") {
		t.Fatalf("memo = %q", rev.Memo)
	}

	again, err := f.ledger.GetEntry(ctx, p, orig.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Memo != orig.Memo || len(again.Lines) != 2 || again.Lines[0].Debit != 500000 || again.ReversesID != "" {
		t.Fatalf("the original changed: %+v", again)
	}

	tb, err := f.ledger.TrialBalance(ctx, p, mustDate(t, "2026-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range tb.Lines {
		if l.Debit != l.Credit {
			t.Fatalf("account %s nets to %v debit vs %v credit; a reversed entry must net to zero", l.Code, l.Debit, l.Credit)
		}
	}
}

func TestADR_0004_ReversalRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, _ := ledgerPrincipal(t, f, "led-revno")
	orig := mustPost(t, f, p, billing(100, "2026-11-07", t))
	on := mustDate(t, "2026-11-08")

	reverse := func(id, reason string) (ledger.PostedEntry, error) {
		var out ledger.PostedEntry
		err := db.WithTenant(ctx, f.app, p.TenantID, func(tx pgx.Tx) error {
			var err error
			out, err = f.ledger.Reverse(ctx, tx, p, id, reason, on)
			return err
		})
		return out, err
	}

	if _, err := reverse(orig.ID, "no"); errCode(t, err) != "ledger.reason_invalid" {
		t.Fatalf("short reason: %v", err)
	}
	if _, err := reverse("5c9c6c26-e0e4-4938-af5c-f6fc35688270", "some reason"); errCode(t, err) != "ledger.entry_not_found" {
		t.Fatalf("unknown entry: %v", err)
	}
	if _, err := reverse("not-an-id", "some reason"); errCode(t, err) != "ledger.entry_not_found" {
		t.Fatalf("bad id: %v", err)
	}
	rev, err := reverse(orig.ID, "first reversal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reverse(orig.ID, "second reversal"); errCode(t, err) != "ledger.already_reversed" {
		t.Fatalf("second reversal: %v", err)
	}
	if _, err := reverse(rev.ID, "reverse the reversal"); errCode(t, err) != "ledger.cannot_reverse_a_reversal" {
		t.Fatalf("reversing a reversal: %v", err)
	}
}

// The trial balance only counts entries on or before the report date.
func TestADR_0004_TrialBalanceHonoursTheReportDate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, _ := ledgerPrincipal(t, f, "led-asof")
	mustPost(t, f, p, billing(100000, "2026-10-01", t))
	mustPost(t, f, p, billing(200000, "2026-11-01", t))

	for _, c := range []struct {
		asOf string
		want money.Money
	}{
		{"2026-09-30", 0},
		{"2026-10-01", 100000},
		{"2026-10-31", 100000},
		{"2026-11-01", 300000},
	} {
		tb, err := f.ledger.TrialBalance(ctx, p, mustDate(t, c.asOf))
		if err != nil {
			t.Fatal(err)
		}
		if tb.TotalDebit != c.want || !tb.Balanced {
			t.Fatalf("as of %s: debit %v balanced %v, want %v", c.asOf, tb.TotalDebit, tb.Balanced, c.want)
		}
	}
}

// Newest first, no overlap, ends with no cursor, and bad inputs are rejected.
func TestREQ_API_LedgerPaging(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	p, _ := ledgerPrincipal(t, f, "led-page")
	for range 5 {
		mustPost(t, f, p, billing(100, "2026-11-07", t))
	}

	var seen []int64
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		page, err := f.ledger.ListEntries(ctx, p, 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range page.Items {
			seen = append(seen, e.EntryNo)
			if len(e.Lines) != 2 {
				t.Fatalf("entry %d has %d lines", e.EntryNo, len(e.Lines))
			}
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if !slices.Equal(seen, []int64{5, 4, 3, 2, 1}) {
		t.Fatalf("entries = %v, want 5 4 3 2 1", seen)
	}

	if _, err := f.ledger.ListEntries(ctx, p, 2, "!!"); errCode(t, err) != "ledger.cursor_invalid" {
		t.Fatalf("bad cursor: %v", err)
	}
	for _, limit := range []int{0, 201} {
		if _, err := f.ledger.ListEntries(ctx, p, limit, ""); errCode(t, err) != "ledger.limit_invalid" {
			t.Fatalf("limit %d: %v", limit, err)
		}
	}
}

// Tenant isolation for the ledger: B cannot read A's entries or accounts, or post to A's account.
func TestRLS_LedgerIsolatedBetweenTenants(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pA, tenantA := ledgerPrincipal(t, f, "led-a")
	pB, tenantB := ledgerPrincipal(t, f, "led-b")
	entryA := mustPost(t, f, pA, billing(100, "2026-11-07", t))
	mustPost(t, f, pB, billing(100, "2026-11-07", t)) // B has its own chart

	if _, err := f.ledger.GetEntry(ctx, pB, entryA.ID); errCode(t, err) != "ledger.entry_not_found" {
		t.Fatalf("B read A's entry: %v", err)
	}
	page, err := f.ledger.ListEntries(ctx, pB, 200, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range page.Items {
		if e.ID == entryA.ID {
			t.Fatal("B can list A's entry")
		}
	}
	if len(page.Items) != 1 {
		t.Fatalf("B should see exactly its own entry, saw %d", len(page.Items))
	}
	err = db.WithTenant(ctx, f.app, tenantB, func(tx pgx.Tx) error {
		_, err := f.ledger.Reverse(ctx, tx, pB, entryA.ID, "attack", mustDate(t, "2026-11-08"))
		return err
	})
	if errCode(t, err) != "ledger.entry_not_found" {
		t.Fatalf("B reversed A's entry: %v", err)
	}

	// Raw SQL: B's entry cannot point at A's account (composite foreign key).
	var accountA string
	if err := f.admin.QueryRow(ctx, `SELECT id::text FROM accounts WHERE tenant_id = $1 AND code = '1000'`, tenantA).Scan(&accountA); err != nil {
		t.Fatal(err)
	}
	err = db.WithTenant(ctx, f.app, tenantB, func(tx pgx.Tx) error {
		var entryID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO journal_entries (tenant_id, entry_no, occurred_on, memo, source_type)
			VALUES ($1, 500, '2026-11-07', 'attack', 'test') RETURNING id::text`, tenantB).Scan(&entryID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO journal_lines (tenant_id, entry_id, line_no, account_id, debit, credit)
			VALUES ($1, $2, 1, $3, 10, 0)`, tenantB, entryID, accountA)
		return err
	})
	if pgCode(err) != "23503" {
		t.Fatalf("want a foreign key violation (23503), got %v", err)
	}
}
