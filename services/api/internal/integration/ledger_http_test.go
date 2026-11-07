package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
)

type apiMoney struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// The ledger endpoints need a token (401) and their named permission (403).
func TestREQ_API_LedgerEndpointsRequireAuthAndPermission(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "led-authz")
	session := signIn(t, f, mobile, slug)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger), f.iam)
	if _, err := f.admin.Exec(ctx, `DELETE FROM role_assignments WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		"/api/v1/accounts",
		"/api/v1/journal-entries",
		"/api/v1/journal-entries/5c9c6c26-e0e4-4938-af5c-f6fc35688270",
		"/api/v1/reports/trial-balance",
	}
	for _, path := range paths {
		t.Run(path+" without a token is 401", func(t *testing.T) {
			if rec := do(t, h, http.MethodGet, path, "", ""); rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d, want 401", rec.Code)
			}
		})
		t.Run(path+" without the permission is 403", func(t *testing.T) {
			if rec := do(t, h, http.MethodGet, path, session, ""); rec.Code != http.StatusForbidden {
				t.Fatalf("status %d, want 403: %s", rec.Code, rec.Body)
			}
		})
	}
}

// Money crosses the API as integer paisa plus a currency, never a decimal or a string.
func TestREQ_API_LedgerReadsOverHTTP(t *testing.T) {
	f := newFixture(t)
	slug, mobile, _ := newTenant(t, f, "led-http")
	p := principalFor(t, f, slug, mobile)
	token := signIn(t, f, mobile, slug)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger), f.iam)

	// A school that has never posted has no accounts and an empty, balanced trial balance.
	rec := do(t, h, http.MethodGet, "/api/v1/accounts", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("accounts: %d %s", rec.Code, rec.Body)
	}
	var accounts struct {
		Items []struct{ Code string } `json:"items"`
	}
	mustJSON(t, rec.Body.Bytes(), &accounts)
	if len(accounts.Items) != 0 {
		t.Fatalf("a school that never posted should have no accounts, got %d", len(accounts.Items))
	}

	first := mustPost(t, f, p, billing(1250050, "2026-11-07", t)) // PKR 12,500.50
	mustPost(t, f, p, billing(100000, "2026-11-08", t))

	rec = do(t, h, http.MethodGet, "/api/v1/accounts", token, "")
	mustJSON(t, rec.Body.Bytes(), &accounts)
	if len(accounts.Items) != 7 || accounts.Items[0].Code != "1000" {
		t.Fatalf("want the 7 system accounts starting at 1000, got %+v", accounts.Items)
	}

	// One entry: amounts are integers, with the currency beside them.
	rec = do(t, h, http.MethodGet, "/api/v1/journal-entries/"+first.ID, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("entry: %d %s", rec.Code, rec.Body)
	}
	var entry struct {
		EntryNo int64  `json:"entry_no"`
		Memo    string `json:"memo"`
		Lines   []struct {
			AccountCode string   `json:"account_code"`
			Debit       apiMoney `json:"debit"`
			Credit      apiMoney `json:"credit"`
		} `json:"lines"`
	}
	mustJSON(t, rec.Body.Bytes(), &entry)
	if entry.EntryNo != 1 || len(entry.Lines) != 2 ||
		entry.Lines[0].AccountCode != "1200" || entry.Lines[0].Debit != (apiMoney{1250050, "PKR"}) ||
		entry.Lines[1].AccountCode != "4000" || entry.Lines[1].Credit != (apiMoney{1250050, "PKR"}) {
		t.Fatalf("entry is wrong: %s", rec.Body)
	}

	// The trial balance as of the first day counts only the first entry; later, both.
	var tb struct {
		AsOf        string   `json:"as_of"`
		Balanced    bool     `json:"balanced"`
		TotalDebit  apiMoney `json:"total_debit"`
		TotalCredit apiMoney `json:"total_credit"`
	}
	rec = do(t, h, http.MethodGet, "/api/v1/reports/trial-balance?as_of=2026-11-07", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("trial balance: %d %s", rec.Code, rec.Body)
	}
	mustJSON(t, rec.Body.Bytes(), &tb)
	if !tb.Balanced || tb.TotalDebit != (apiMoney{1250050, "PKR"}) || tb.TotalCredit != tb.TotalDebit || tb.AsOf != "2026-11-07" {
		t.Fatalf("trial balance as of 2026-11-07 is wrong: %s", rec.Body)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/reports/trial-balance?as_of=2026-11-08", token, "")
	mustJSON(t, rec.Body.Bytes(), &tb)
	if tb.TotalDebit.Amount != 1350050 {
		t.Fatalf("both entries should total 1350050, got %d", tb.TotalDebit.Amount)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/reports/trial-balance?as_of=not-a-date", token, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("a bad date should be 400, got %d", rec.Code)
	}

	// Paging over HTTP.
	rec = do(t, h, http.MethodGet, "/api/v1/journal-entries?limit=1", token, "")
	var page struct {
		Items []struct {
			EntryNo int64 `json:"entry_no"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	mustJSON(t, rec.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Items[0].EntryNo != 2 || page.NextCursor == nil {
		t.Fatalf("page 1 is wrong: %s", rec.Body)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/journal-entries?limit=1&cursor="+*page.NextCursor, token, "")
	page.NextCursor = nil // json.Unmarshal does not clear fields that are absent from the new body
	page.Items = nil
	mustJSON(t, rec.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Items[0].EntryNo != 1 || page.NextCursor != nil {
		t.Fatalf("page 2 is wrong: %s", rec.Body)
	}
}

// Another school's entry is a 404, never a 403, and the id is not confirmed to exist.
func TestREQ_API_LedgerEntryOfAnotherTenantIs404(t *testing.T) {
	f := newFixture(t)
	slugA, mobileA, _ := newTenant(t, f, "led-ha")
	slugB, mobileB, _ := newTenant(t, f, "led-hb")
	pA := principalFor(t, f, slugA, mobileA)
	tokenB := signIn(t, f, mobileB, slugB)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger), f.iam)
	entry := mustPost(t, f, pA, billing(100, "2026-11-07", t))

	if rec := do(t, h, http.MethodGet, "/api/v1/journal-entries/"+entry.ID, tokenB, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}

// ADR-0014 / DEC-011: no HTTP route can create, change or delete money. The ledger resources
// accept GET only; every other method is refused and nothing is stored.
func TestADR_0014_NoHTTPRouteCanPostOrChangeMoney(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "led-nowrite")
	p := principalFor(t, f, slug, mobile)
	token := signIn(t, f, mobile, slug)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger), f.iam)
	entry := mustPost(t, f, p, billing(100, "2026-11-07", t))

	body := `{"memo":"sneaky","source_type":"x","occurred_on":"2026-11-07","lines":[]}`
	attempts := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/journal-entries"},
		{http.MethodPut, "/api/v1/journal-entries/" + entry.ID},
		{http.MethodPatch, "/api/v1/journal-entries/" + entry.ID},
		{http.MethodDelete, "/api/v1/journal-entries/" + entry.ID},
		{http.MethodPost, "/api/v1/accounts"},
		{http.MethodDelete, "/api/v1/accounts"},
		{http.MethodPost, "/api/v1/reports/trial-balance"},
	}
	for _, a := range attempts {
		t.Run(a.method+" "+a.path, func(t *testing.T) {
			rec := do(t, h, a.method, a.path, token, body)
			if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
				t.Fatalf("status %d, want 405 or 404: %s", rec.Code, rec.Body)
			}
		})
	}

	var n int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("after the attempts there should still be exactly 1 entry: %v, %d", err, n)
	}
}
