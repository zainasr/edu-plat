package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
)

// The challan flow over HTTP: generate (preview and real), release, read, list, void, with Money
// as {amount, currency} and the stable error codes. 401/403 for every endpoint are proved by
// TestINV_EveryOperationRequiresAuthAndAPermission.
func TestREQ_API_ChallanFlowOverHTTP(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "chal-http")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	month := thisMonth().Format("2006-01-02")

	// A due date before the month is a 400 with the field named.
	rec := do(t, h, http.MethodPost, "/api/v1/challans/generate", token, `{"month":"`+month+`","due_date":"2000-01-01"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"fees.generate_invalid"`) || !strings.Contains(rec.Body.String(), `"due_date"`) {
		t.Fatalf("bad due date: %d %s", rec.Code, rec.Body)
	}

	type result struct {
		Generated int  `json:"generated"`
		DryRun    bool `json:"dry_run"`
		Items     []struct {
			Outcome   string    `json:"outcome"`
			Total     moneyJSON `json:"total"`
			ChallanID string    `json:"challan_id"`
		} `json:"items"`
	}
	var prev result
	rec = do(t, h, http.MethodPost, "/api/v1/challans/generate", token, `{"month":"`+month+`","dry_run":true}`)
	mustJSON(t, rec.Body.Bytes(), &prev)
	if rec.Code != http.StatusOK || !prev.DryRun || prev.Generated != 0 || len(prev.Items) != 1 ||
		prev.Items[0].Outcome != "will_generate" || prev.Items[0].Total != (moneyJSON{500000, "PKR"}) {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}

	var made result
	rec = do(t, h, http.MethodPost, "/api/v1/challans/generate", token, `{"month":"`+month+`"}`)
	mustJSON(t, rec.Body.Bytes(), &made)
	if rec.Code != http.StatusOK || made.Generated != 1 || made.Items[0].ChallanID == "" {
		t.Fatalf("generate: %d %s", rec.Code, rec.Body)
	}
	id := made.Items[0].ChallanID

	rec = do(t, h, http.MethodPost, "/api/v1/challans/release", token, `{"challan_ids":["`+id+`"]}`)
	var released struct {
		Items []struct {
			Status     string    `json:"status"`
			Total      moneyJSON `json:"total"`
			ChallanNo  int64     `json:"challan_no"`
			ConsumerNo string    `json:"consumer_no"`
		} `json:"items"`
	}
	mustJSON(t, rec.Body.Bytes(), &released)
	if rec.Code != http.StatusOK || len(released.Items) != 1 || released.Items[0].Status != "released" ||
		released.Items[0].ChallanNo != 1 || len(released.Items[0].ConsumerNo) != 14 || released.Items[0].Total.Amount != 500000 {
		t.Fatalf("release: %d %s", rec.Code, rec.Body)
	}

	// Releasing again is a 409 with the stable code; voiding it is too.
	rec = do(t, h, http.MethodPost, "/api/v1/challans/release", token, `{"challan_ids":["`+id+`"]}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"challans.not_draft"`) {
		t.Fatalf("second release: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodPost, "/api/v1/challans/"+id+"/void", token, `{"reason":"oops"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("void released: %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/challans/"+id, token, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"fee_type_name":"Tuition"`) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/challans?status=released&month="+month, token, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), id) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/challans/5c9c6c26-e0e4-4938-af5c-f6fc35688270", token, "")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"challans.not_found"`) {
		t.Fatalf("unknown id: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodPost, "/api/v1/challans/release", token, `{"challan_ids":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty release: %d %s", rec.Code, rec.Body)
	}
}
