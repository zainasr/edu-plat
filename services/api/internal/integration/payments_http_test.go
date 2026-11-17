package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
)

// The counter flow over HTTP: the key is required, a retry returns the same receipt, Money is
// {amount, currency}, and refusals carry stable codes.
func TestREQ_API_PaymentsOverHTTP(t *testing.T) {
	f := newFixture(t)
	w := newPayWorld(t, f, "pay-http")
	token := signInAs(t, f, w.feeWorld)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))

	body := `{"family_id":"` + w.familyID + `","amount":{"amount":200000,"currency":"PKR"},"method":"cash","account_code":"1000","reference":"slip 9"}`

	// Without the key: 400 with the code, after the permission check.
	if rec := keyed(t, h, "/api/v1/payments", token, "", body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "idempotency.key_required") {
		t.Fatalf("no key: %d %s", rec.Code, rec.Body)
	}
	first := keyed(t, h, "/api/v1/payments", token, "counter-pay-0000001", body)
	var got struct {
		ReceiptNo   string    `json:"receipt_no"`
		Status      string    `json:"status"`
		Amount      moneyJSON `json:"amount"`
		AccountCode string    `json:"account_code"`
		Allocations []struct {
			Amount moneyJSON `json:"amount"`
		} `json:"allocations"`
	}
	mustJSON(t, first.Body.Bytes(), &got)
	if first.Code != http.StatusCreated || got.Status != "recorded" || got.Amount != (moneyJSON{200000, "PKR"}) || got.AccountCode != "1000" ||
		len(got.Allocations) != 1 || !strings.HasPrefix(got.ReceiptNo, "RCP-") {
		t.Fatalf("take payment: %d %s", first.Code, first.Body)
	}
	// The retry after a lost response: the same receipt, and only one payment exists.
	retry := keyed(t, h, "/api/v1/payments", token, "counter-pay-0000001", body)
	if retry.Code != http.StatusCreated || retry.Body.String() != first.Body.String() || retry.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("retry: %d %s", retry.Code, retry.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/payments?family_id="+w.familyID, token, ""); !strings.Contains(rec.Body.String(), got.ReceiptNo) || strings.Count(rec.Body.String(), `"receipt_no"`) != 1 {
		t.Fatalf("one payment, listed once: %s", rec.Body)
	}
	// A different key is a new payment: this one is refused (more than the 300,000 still owed).
	over := strings.Replace(body, "200000", "300001", 1)
	if rec := keyed(t, h, "/api/v1/payments", token, "counter-pay-0000002", over); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "payments.exceeds_dues") {
		t.Fatalf("over the dues: %d %s", rec.Code, rec.Body)
	}
	// Dues and the challan view over HTTP.
	if rec := do(t, h, http.MethodGet, "/api/v1/families/"+w.familyID+"/dues", token, ""); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"total_due":{"amount":300000,"currency":"PKR"}`) {
		t.Fatalf("dues: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/challans?status=released", token, ""); !strings.Contains(rec.Body.String(), `"payment_status":"partial"`) ||
		!strings.Contains(rec.Body.String(), `"balance":{"amount":300000,"currency":"PKR"}`) {
		t.Fatalf("challan paid and balance: %s", rec.Body)
	}
	// Void over HTTP.
	var id struct {
		ID string `json:"id"`
	}
	mustJSON(t, first.Body.Bytes(), &id)
	if rec := do(t, h, http.MethodPost, "/api/v1/payments/"+id.ID+"/void", token, `{"reason":"wrong family"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"voided"`) {
		t.Fatalf("void: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/payments/"+id.ID+"/void", token, `{"reason":"again"}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "payments.already_voided") {
		t.Fatalf("void twice: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/payments/5c9c6c26-e0e4-4938-af5c-f6fc35688270", token, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown payment: %d", rec.Code)
	}
	if rec := keyed(t, h, "/api/v1/payments", token, "counter-pay-0000003", strings.Replace(body, `"currency":"PKR"`, `"currency":"USD"`, 1)); rec.Code != http.StatusBadRequest {
		t.Fatalf("a non-PKR amount: %d %s", rec.Code, rec.Body)
	}
}

// Advance, refund and defaulters over HTTP.
func TestREQ_API_AdvanceRefundAndDefaultersOverHTTP(t *testing.T) {
	f := newFixture(t)
	w := newPayWorld(t, f, "adv-http")
	token := signInAs(t, f, w.feeWorld)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam, api.WithIdempotency(f.app))
	body := func(amount int, extra string) string {
		return `{"family_id":"` + w.familyID + `","amount":{"amount":` + itoaInt(amount) + `,"currency":"PKR"},"method":"cash","account_code":"1000"` + extra + `}`
	}

	// Above what is owed: refused unless the extra is to be held as advance.
	if rec := keyed(t, h, "/api/v1/payments", token, "adv-key-00000001", body(800000, "")); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "payments.exceeds_dues") {
		t.Fatalf("without the flag: %d %s", rec.Code, rec.Body)
	}
	rec := keyed(t, h, "/api/v1/payments", token, "adv-key-00000002", body(800000, `,"hold_excess_as_advance":true`))
	var pay struct {
		ID               string    `json:"id"`
		Advance          moneyJSON `json:"advance"`
		AdvanceAvailable moneyJSON `json:"advance_available"`
	}
	mustJSON(t, rec.Body.Bytes(), &pay)
	if rec.Code != http.StatusCreated || pay.Advance.Amount != 300000 || pay.AdvanceAvailable.Amount != 300000 {
		t.Fatalf("hold the extra: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/families/"+w.familyID+"/dues", token, ""); !strings.Contains(rec.Body.String(), `"advance_available":{"amount":300000,"currency":"PKR"}`) {
		t.Fatalf("dues show the advance: %s", rec.Body)
	}

	refund := `{"amount":{"amount":100000,"currency":"PKR"},"account_code":"1000","reason":"parent asked for it back"}`
	first := keyed(t, h, "/api/v1/payments/"+pay.ID+"/refund", token, "refund-key-0000001", refund)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"advance_refunded":{"amount":100000,"currency":"PKR"}`) ||
		!strings.Contains(first.Body.String(), `"advance_available":{"amount":200000,"currency":"PKR"}`) {
		t.Fatalf("refund: %d %s", first.Code, first.Body)
	}
	// The retry replays instead of paying back twice.
	retry := keyed(t, h, "/api/v1/payments/"+pay.ID+"/refund", token, "refund-key-0000001", refund)
	if retry.Code != http.StatusOK || retry.Body.String() != first.Body.String() || retry.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("retry: %d %s", retry.Code, retry.Body)
	}
	var refunds int
	_ = f.admin.QueryRow(context.Background(), `SELECT count(*) FROM advance_refunds WHERE payment_id = $1`, pay.ID).Scan(&refunds)
	if refunds != 1 {
		t.Fatalf("one refund, got %d", refunds)
	}
	// More than is held: 409 with the code. Voiding after a refund: 409.
	tooMuch := strings.Replace(refund, "100000", "200001", 1)
	if rec := keyed(t, h, "/api/v1/payments/"+pay.ID+"/refund", token, "refund-key-0000002", tooMuch); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "payments.refund_exceeds_advance") {
		t.Fatalf("too much: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/payments/"+pay.ID+"/void", token, `{"reason":"too late now"}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "payments.advance_in_use") {
		t.Fatalf("void after refund: %d %s", rec.Code, rec.Body)
	}
	// Defaulters: JSON shape, and a bad window.
	if rec := do(t, h, http.MethodGet, "/api/v1/defaulters?min_days=0", token, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("min_days=0: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/defaulters?min_days=1", token, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items"`) {
		t.Fatalf("defaulters: %d %s", rec.Code, rec.Body)
	}
}

func itoaInt(n int) string { return fmt.Sprintf("%d", n) }
