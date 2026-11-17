package integration

import (
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
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees), f.iam, api.WithIdempotency(f.app))

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
