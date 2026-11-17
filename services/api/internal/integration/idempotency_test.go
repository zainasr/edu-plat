package integration

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/fees"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/idempotency"
	"github.com/zainasr/edu-plat/services/api/internal/platform/idempotency/store/gen"
)

func keyed(t *testing.T, h http.Handler, path, token, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func journalEntries(t *testing.T, f *fixture, tenantID string) int {
	t.Helper()
	var n int
	if err := f.admin.QueryRow(context.Background(), `SELECT count(*) FROM journal_entries WHERE tenant_id = $1 AND source_type = 'challan.released'`, tenantID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// REQ-API-010: a retried release returns the first answer and posts nothing twice. Without a key the
// second release is still refused as before (the key is optional).
func TestREQ_API_010_ReleaseRetryReplaysTheOriginalResponse(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "idem-rel")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	id := itemFor(t, w.generate(t, f, thisMonth(), false), w.studentID).ChallanID
	body := `{"challan_ids":["` + id + `"]}`
	const key = "release-2026-run-0001"

	first := keyed(t, h, "/api/v1/challans/release", token, key, body)
	if first.Code != http.StatusOK || first.Header().Get("Idempotent-Replay") != "" {
		t.Fatalf("first: %d %s", first.Code, first.Body)
	}
	second := keyed(t, h, "/api/v1/challans/release", token, key, body)
	if second.Code != http.StatusOK || second.Body.String() != first.Body.String() || second.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("replay: %d replay=%q\n got %s\nwant %s", second.Code, second.Header().Get("Idempotent-Replay"), second.Body, first.Body)
	}
	if n := journalEntries(t, f, w.tenantID); n != 1 {
		t.Fatalf("a replay must not post again: %d receivable entries", n)
	}
	// No key: the old behaviour, a plain 409.
	if rec := keyed(t, h, "/api/v1/challans/release", token, "", body); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "challans.not_draft") {
		t.Fatalf("without a key: %d %s", rec.Code, rec.Body)
	}
	// A different key is a new request: it is refused on its merits.
	if rec := keyed(t, h, "/api/v1/challans/release", token, "release-2026-run-0002", body); rec.Code != http.StatusConflict {
		t.Fatalf("new key: %d %s", rec.Code, rec.Body)
	}
}

// The same key with a different body, or on a different endpoint, is a mistake: 422.
func TestREQ_API_010_KeyReuseIsRefused(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "idem-reuse")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	month := thisMonth().Format("2006-01-02")
	const key = "shared-key-12345678"

	if rec := keyed(t, h, "/api/v1/challans/generate", token, key, `{"month":"`+month+`","dry_run":true}`); rec.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rec.Code, rec.Body)
	}
	for name, c := range map[string]struct{ path, body string }{
		"different body":     {"/api/v1/challans/generate", `{"month":"` + month + `","dry_run":false}`},
		"different endpoint": {"/api/v1/challans/release", `{"challan_ids":["5c9c6c26-e0e4-4938-af5c-f6fc35688270"]}`},
	} {
		rec := keyed(t, h, c.path, token, key, c.body)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"idempotency.key_reused"`) {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	// And nothing ran for the refused ones: still no challan.
	if page, _ := f.fees.ListChallans(context.Background(), w.p, fees.ChallanFilter{}); len(page.Items) != 0 {
		t.Fatalf("a refused request must not run: %d challans", len(page.Items))
	}
}

// Six identical requests at once: one runs, the rest either wait their turn with 409 or replay.
func TestREQ_API_010_ConcurrentRetriesRunOnce(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "idem-race")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	id := itemFor(t, w.generate(t, f, thisMonth(), false), w.studentID).ChallanID
	body := `{"challan_ids":["` + id + `"]}`

	const n = 6
	recs := make([]*httptest.ResponseRecorder, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recs[i] = keyed(t, h, "/api/v1/challans/release", token, "race-key-0000001", body)
		}()
	}
	wg.Wait()
	ok, original, fresh := 0, "", 0
	for _, r := range recs {
		switch {
		case r.Code == http.StatusOK:
			ok++
			if r.Header().Get("Idempotent-Replay") == "" {
				fresh++
			}
			if original == "" {
				original = r.Body.String()
			} else if r.Body.String() != original {
				t.Fatalf("every 200 must carry the same body")
			}
		case r.Code == http.StatusConflict && strings.Contains(r.Body.String(), "idempotency.in_progress"):
		default:
			t.Fatalf("unexpected: %d %s", r.Code, r.Body)
		}
	}
	if ok < 1 || fresh != 1 {
		t.Fatalf("exactly one request runs (%d did) and the others replay or wait", fresh)
	}
	if n := journalEntries(t, f, w.tenantID); n != 1 {
		t.Fatalf("exactly one release may run: %d entries", n)
	}
}

// A key whose first request is still running gets 409; a key left "in progress" by a crash more
// than 24 hours ago is taken over.
func TestREQ_API_010_InProgressAndExpiredKeys(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "idem-prog")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	id := itemFor(t, w.generate(t, f, thisMonth(), false), w.studentID).ChallanID
	body := `{"challan_ids":["` + id + `"]}`
	// What the middleware stores for this request: the user, the path, the query, then the body.
	sum := sha256.Sum256([]byte(w.p.UserID + "\x00" + "/api/v1/challans/release" + "\x00" + "" + "\x00" + body))
	const key = "crashed-run-0000001"
	if _, err := f.admin.Exec(ctx, `INSERT INTO idempotency_keys (tenant_id, key, endpoint, request_hash) VALUES ($1, $2, 'POST /api/v1/challans/release', $3)`, w.tenantID, key, sum[:]); err != nil {
		t.Fatal(err)
	}

	rec := keyed(t, h, "/api/v1/challans/release", token, key, body)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "idempotency.in_progress") {
		t.Fatalf("a running key: %d %s", rec.Code, rec.Body)
	}
	if journalEntries(t, f, w.tenantID) != 0 {
		t.Fatal("a 409 must not run the request")
	}

	if _, err := f.admin.Exec(ctx, `UPDATE idempotency_keys SET created_at = now() - interval '25 hours' WHERE tenant_id = $1 AND key = $2`, w.tenantID, key); err != nil {
		t.Fatal(err)
	}
	if rec := keyed(t, h, "/api/v1/challans/release", token, key, body); rec.Code != http.StatusOK {
		t.Fatalf("an expired key is taken over: %d %s", rec.Code, rec.Body)
	}
	if journalEntries(t, f, w.tenantID) != 1 {
		t.Fatal("the taken-over request must run once")
	}
}

// Request validation around the key.
func TestREQ_API_010_KeyValidationAndPassThrough(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "idem-val")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))
	body := `{"month":"` + thisMonth().Format("2006-01-02") + `","dry_run":true}`

	if rec := keyed(t, h, "/api/v1/challans/generate", token, "short", body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "idempotency.key_invalid") {
		t.Fatalf("a bad key: %d %s", rec.Code, rec.Body)
	}
	// Anonymous callers still get 401, key or not.
	if rec := keyed(t, h, "/api/v1/challans/generate", "", "valid-key-00000001", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous with a key: %d %s", rec.Code, rec.Body)
	}
	// A key on an endpoint that does not honour it is ignored, not stored.
	rec := keyed(t, h, "/api/v1/fee-types", token, "ignored-key-0000001", `{"code":"lab_fee","name":"Lab","kind":"lab"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("fee type: %d %s", rec.Code, rec.Body)
	}
	var n int
	if err := f.admin.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_keys WHERE tenant_id = $1 AND key = 'ignored-key-0000001'`, w.tenantID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a key on an unlisted endpoint must not be stored: %d (%v)", n, err)
	}
}

// Server errors and panics free the key so a retry runs again; client errors are replayed; schools never
// see each other's keys. A stub handler stands in for the endpoint so the cases can be forced.
func TestREQ_API_010_FailuresFreeTheKeyAndSchoolsAreSeparate(t *testing.T) {
	f := newFixture(t)
	a := newFeeWorld(t, f, "idem-a")
	b := newFeeWorld(t, f, "idem-b")
	var calls atomic.Int32
	var mode atomic.Value // "500" | "panic" | "400" | "ok"
	mode.Store("ok")
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch mode.Load() {
		case "500":
			w.WriteHeader(http.StatusInternalServerError)
		case "panic":
			panic("boom")
		case "400":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"no"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	})
	tenant := a.tenantID
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/challans/release", idempotency.New(f.app, func(*http.Request) (string, string, bool) { return tenant, "user-1", true })(stub))
	call := func(key string) (code int, panicked bool) {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/challans/release", strings.NewReader(`{}`))
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, false
	}

	// 500: not stored, the retry runs again and succeeds, then replays.
	mode.Store("500")
	if code, _ := call("fail-key-00000001"); code != 500 {
		t.Fatalf("stub 500: %d", code)
	}
	mode.Store("ok")
	if code, _ := call("fail-key-00000001"); code != 200 || calls.Load() != 2 {
		t.Fatalf("retry after a 500 must run again: %d, calls %d", code, calls.Load())
	}
	if code, _ := call("fail-key-00000001"); code != 200 || calls.Load() != 2 {
		t.Fatalf("then replay: %d, calls %d", code, calls.Load())
	}

	// Panic: the key is freed.
	mode.Store("panic")
	if _, panicked := call("panic-key-0000001"); !panicked {
		t.Fatal("the panic must keep going up to net/http")
	}
	mode.Store("ok")
	before := calls.Load()
	if code, _ := call("panic-key-0000001"); code != 200 || calls.Load() != before+1 {
		t.Fatalf("retry after a panic must run again: %d", code)
	}

	// 400: a client error did nothing, so the key is freed and a retry runs again (and is not stuck
	// with an old failure, for example a 403 for someone who has since been given the permission).
	mode.Store("400")
	before = calls.Load()
	call("bad-key-000000001")
	mode.Store("ok")
	if code, _ := call("bad-key-000000001"); code != 200 || calls.Load() != before+2 {
		t.Fatalf("a 400 must not be remembered: got %d with %d runs", code, calls.Load()-before)
	}
	if code, _ := call("bad-key-000000001"); code != 200 || calls.Load() != before+2 {
		t.Fatalf("the success is then replayed: %d, runs %d", code, calls.Load()-before)
	}

	// Schools: the same key in school B runs on its own and does not see A's stored response.
	before = calls.Load()
	tenant = b.tenantID
	if code, _ := call("bad-key-000000001"); code != 200 || calls.Load() != before+1 {
		t.Fatalf("school B must not see school A's key: %d", code)
	}
}

// A key belongs to one user. Another user in the same school presenting it gets 422, not the first
// user's response: a replay skips the handler, so it would skip the permission check too.
func TestREQ_API_010_AKeyBelongsToOneUser(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "idem-user")
	owner := signInAs(t, f, w)
	accountant := addStaff(t, f, w.tenantID, w.slug, "accountant")
	receptionist := addStaff(t, f, w.tenantID, w.slug, "receptionist")
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	id := itemFor(t, w.generate(t, f, thisMonth(), false), w.studentID).ChallanID
	body := `{"challan_ids":["` + id + `"]}`
	const key = "owner-only-key-001"

	if rec := keyed(t, h, "/api/v1/challans/release", owner, key, body); rec.Code != http.StatusOK {
		t.Fatalf("owner: %d %s", rec.Code, rec.Body)
	}
	for name, token := range map[string]string{"accountant (may release)": accountant, "receptionist (may not)": receptionist} {
		rec := keyed(t, h, "/api/v1/challans/release", token, key, body)
		if rec.Code != http.StatusUnprocessableEntity || strings.Contains(rec.Body.String(), `"challan_no"`) {
			t.Fatalf("%s must not get the owner's stored response: %d %s", name, rec.Code, rec.Body)
		}
	}

	// And a 403 is never remembered: the receptionist's refused request leaves the key free.
	const k2 = "refused-then-ok-001"
	if rec := keyed(t, h, "/api/v1/challans/release", receptionist, k2, body); rec.Code != http.StatusForbidden {
		t.Fatalf("receptionist: %d %s", rec.Code, rec.Body)
	}
	var stored int
	_ = f.admin.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_keys WHERE tenant_id = $1 AND key = $2 AND status = 'done'`, w.tenantID, k2).Scan(&stored)
	if stored != 0 {
		t.Fatal("a 403 must not be stored as a done response")
	}
}

// The path is part of the request: the same key and body on two different challans is not a replay.
func TestREQ_API_010_SameKeyOnTwoChallansIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "idem-void")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	second := w.addStudent(t, f, "Sara Khan")
	res := w.generate(t, f, thisMonth(), false)
	a, b := itemFor(t, res, w.studentID).ChallanID, itemFor(t, res, second).ChallanID
	const key = "void-key-0000000001"
	body := `{"reason":"duplicate"}`

	if rec := keyed(t, h, "/api/v1/challans/"+a+"/void", token, key, body); rec.Code != http.StatusOK {
		t.Fatalf("void A: %d %s", rec.Code, rec.Body)
	}
	if rec := keyed(t, h, "/api/v1/challans/"+b+"/void", token, key, body); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("the same key on challan B must be refused, not replay A's answer: %d %s", rec.Code, rec.Body)
	}
	if c, _ := f.fees.GetChallan(ctx, w.p, b); c.Status != "draft" {
		t.Fatalf("B must be untouched: %+v", c)
	}
	// A's own retry still replays.
	if rec := keyed(t, h, "/api/v1/challans/"+a+"/void", token, key, body); rec.Code != http.StatusOK || rec.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("A's retry: %d %s", rec.Code, rec.Body)
	}
}

// Only the run that owns a key may finish it. A run that lost the key (it was taken over after it
// expired) cannot store its answer over the new owner's.
func TestREQ_API_010_OnlyTheOwnerFinishesAKey(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "idem-owner")
	tenant, _ := db.ParseUUID(w.tenantID)
	claim := func() gen.ClaimKeyRow {
		var row gen.ClaimKeyRow
		err := db.WithTenant(ctx, f.app, w.tenantID, func(tx pgx.Tx) error {
			var err error
			row, err = gen.New(tx).ClaimKey(ctx, gen.ClaimKeyParams{TenantID: tenant, Key: "owner-test-key-001", Endpoint: "POST /x", RequestHash: make([]byte, 32)})
			return err
		})
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		return row
	}
	complete := func(c gen.ClaimKeyRow, body string) int64 {
		var n int64
		err := db.WithTenant(ctx, f.app, w.tenantID, func(tx pgx.Tx) error {
			var err error
			n, err = gen.New(tx).CompleteKey(ctx, gen.CompleteKeyParams{
				TenantID: tenant, ID: c.ID, Owner: c.Owner, ResponseCode: pgtype.Int4{Int32: 200, Valid: true}, ResponseBody: []byte(body)})
			return err
		})
		if err != nil {
			t.Fatalf("complete: %v", err)
		}
		return n
	}
	first := claim()
	if _, err := f.admin.Exec(ctx, `UPDATE idempotency_keys SET created_at = now() - interval '25 hours' WHERE tenant_id = $1 AND key = 'owner-test-key-001'`, w.tenantID); err != nil {
		t.Fatal(err)
	}
	second := claim() // takes the expired key over
	if first.Owner == second.Owner {
		t.Fatal("a takeover must mint a new owner")
	}
	if n := complete(first, "stale"); n != 0 {
		t.Fatal("the old owner must not be able to finish the key")
	}
	if n := complete(second, "fresh"); n != 1 {
		t.Fatal("the current owner must be able to finish the key")
	}
	var body string
	_ = f.admin.QueryRow(ctx, `SELECT convert_from(response_body, 'UTF8') FROM idempotency_keys WHERE tenant_id = $1 AND key = 'owner-test-key-001'`, w.tenantID).Scan(&body)
	if body != "fresh" {
		t.Fatalf("stored body %q, want the current owner's", body)
	}
}
