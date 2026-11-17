package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
)

type moneyJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// Every fee endpoint needs a token (401) and its named permission (403).
func TestREQ_API_FeeEndpointsRequireAuthAndPermission(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "fee-authz")
	session := signIn(t, f, mobile, slug)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)
	if _, err := f.admin.Exec(ctx, `DELETE FROM role_assignments WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}

	const id = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	line := `{"fee_type_id":"` + id + `","effective_from":"2026-04-01","amount":{"amount":1,"currency":"PKR"},"frequency":"monthly"}`
	override := `{"fee_type_id":"` + id + `","effective_from":"2026-04-01","reason":"because","amount":{"amount":1,"currency":"PKR"}}`
	endpoints := []struct{ name, method, path, body string }{
		{"create fee type", http.MethodPost, "/api/v1/fee-types", `{"code":"lab_fee","name":"Lab","kind":"lab"}`},
		{"list fee types", http.MethodGet, "/api/v1/fee-types", ""},
		{"archive fee type", http.MethodPost, "/api/v1/fee-types/" + id + "/archive", ""},
		{"set plan line", http.MethodPost, "/api/v1/fee-plans/" + id + "/lines", line},
		{"get plan", http.MethodGet, "/api/v1/fee-plans/" + id, ""},
		{"plan history", http.MethodGet, "/api/v1/fee-plans/" + id + "/history", ""},
		{"set override", http.MethodPost, "/api/v1/students/" + id + "/fee-overrides", override},
		{"list overrides", http.MethodGet, "/api/v1/students/" + id + "/fee-overrides", ""},
		{"fee structure", http.MethodGet, "/api/v1/students/" + id + "/fee-structure", ""},
	}
	for _, e := range endpoints {
		t.Run(e.name+" without a token is 401", func(t *testing.T) {
			if rec := do(t, h, e.method, e.path, "", e.body); rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d, want 401: %s", rec.Code, rec.Body)
			}
		})
		t.Run(e.name+" without the permission is 403", func(t *testing.T) {
			if rec := do(t, h, e.method, e.path, session, e.body); rec.Code != http.StatusForbidden {
				t.Fatalf("status %d, want 403: %s", rec.Code, rec.Body)
			}
		})
	}
}

// The whole fee setup flow over HTTP, with Money as {amount, currency} in both directions.
func TestREQ_API_FeeSetupFlowOverHTTP(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "fee-http")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)
	tuition := w.types["tuition"]

	// 1. The suggested types are there.
	rec := do(t, h, http.MethodGet, "/api/v1/fee-types", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list types: %d %s", rec.Code, rec.Body)
	}
	var types struct {
		Items []struct {
			Code      string `json:"code"`
			IsDeposit bool   `json:"is_deposit"`
		} `json:"items"`
	}
	mustJSON(t, rec.Body.Bytes(), &types)
	if len(types.Items) != 6 {
		t.Fatalf("want 6 suggested types, got %d", len(types.Items))
	}

	// 2. Set a plan line. Any day in April is stored as the 1st.
	line := `{"fee_type_id":"` + tuition + `","effective_from":"2026-04-17",
	         "amount":{"amount":500000,"currency":"PKR"},"frequency":"monthly"}`
	rec = do(t, h, http.MethodPost, "/api/v1/fee-plans/"+w.classID+"/lines", token, line)
	if rec.Code != http.StatusCreated {
		t.Fatalf("set plan line: %d %s", rec.Code, rec.Body)
	}
	var created struct {
		EffectiveFrom string    `json:"effective_from"`
		Amount        moneyJSON `json:"amount"`
		Frequency     string    `json:"frequency"`
	}
	mustJSON(t, rec.Body.Bytes(), &created)
	if created.EffectiveFrom != "2026-04-01" || created.Amount != (moneyJSON{500000, "PKR"}) || created.Frequency != "monthly" {
		t.Fatalf("created line is wrong: %s", rec.Body)
	}

	// 3. Read the plan for a month.
	rec = do(t, h, http.MethodGet, "/api/v1/fee-plans/"+w.classID+"?month=2026-05-10", token, "")
	var plan struct {
		Month string `json:"month"`
		Lines []struct {
			FeeTypeName string    `json:"fee_type_name"`
			Amount      moneyJSON `json:"amount"`
		} `json:"lines"`
	}
	mustJSON(t, rec.Body.Bytes(), &plan)
	if rec.Code != http.StatusOK || plan.Month != "2026-05-01" || len(plan.Lines) != 1 ||
		plan.Lines[0].FeeTypeName != "Tuition" || plan.Lines[0].Amount.Amount != 500000 {
		t.Fatalf("plan is wrong: %d %s", rec.Code, rec.Body)
	}

	// 4. A student override, then the student's fee structure.
	override := `{"fee_type_id":"` + tuition + `","effective_from":"2026-05-01","reason":"sibling concession",
	             "amount":{"amount":400000,"currency":"PKR"}}`
	rec = do(t, h, http.MethodPost, "/api/v1/students/"+w.studentID+"/fee-overrides", token, override)
	if rec.Code != http.StatusCreated {
		t.Fatalf("set override: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/students/"+w.studentID+"/fee-structure?month=2026-05-01", token, "")
	var structure struct {
		ClassID string `json:"class_id"`
		Lines   []struct {
			Amount     moneyJSON `json:"amount"`
			PlanAmount moneyJSON `json:"plan_amount"`
			Frequency  string    `json:"frequency"`
			Source     string    `json:"source"`
		} `json:"lines"`
	}
	mustJSON(t, rec.Body.Bytes(), &structure)
	if rec.Code != http.StatusOK || structure.ClassID != w.classID || len(structure.Lines) != 1 ||
		structure.Lines[0].Amount != (moneyJSON{400000, "PKR"}) || structure.Lines[0].PlanAmount.Amount != 500000 ||
		structure.Lines[0].Source != "override" {
		t.Fatalf("fee structure is wrong: %d %s", rec.Code, rec.Body)
	}

	// 5. History and overrides lists.
	rec = do(t, h, http.MethodGet, "/api/v1/fee-plans/"+w.classID+"/history", token, "")
	var history struct {
		Items []struct {
			EffectiveFrom string `json:"effective_from"`
		} `json:"items"`
	}
	mustJSON(t, rec.Body.Bytes(), &history)
	if rec.Code != http.StatusOK || len(history.Items) != 1 {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
	if rec = do(t, h, http.MethodGet, "/api/v1/students/"+w.studentID+"/fee-overrides", token, ""); rec.Code != http.StatusOK {
		t.Fatalf("overrides: %d %s", rec.Code, rec.Body)
	}

	// 6. Archive a type: it drops out of the plan from now on.
	if rec = do(t, h, http.MethodPost, "/api/v1/fee-types/"+tuition+"/archive", token, ""); rec.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/fee-plans/"+w.classID+"?month=2026-05-10", token, "")
	mustJSON(t, rec.Body.Bytes(), &plan)
	if len(plan.Lines) != 0 {
		t.Fatalf("an archived type must drop out of the plan: %s", rec.Body)
	}
}

// Money crosses the API as {amount, currency}. Anything else is refused with a clear code.
func TestREQ_API_FeeMoneyInputIsChecked(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "fee-money")
	token := signInAs(t, f, w)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)
	path := "/api/v1/fee-plans/" + w.classID + "/lines"
	typeID := w.types["tuition"]

	tests := []struct {
		name     string
		body     string
		wantCode string
	}{
		{"wrong currency", `{"fee_type_id":"` + typeID + `","effective_from":"2026-04-01","amount":{"amount":1,"currency":"USD"},"frequency":"monthly"}`, ""},
		{"a bare number instead of an object", `{"fee_type_id":"` + typeID + `","effective_from":"2026-04-01","amount":500000,"frequency":"monthly"}`, "request.invalid"},
		{"a decimal amount", `{"fee_type_id":"` + typeID + `","effective_from":"2026-04-01","amount":{"amount":5000.5,"currency":"PKR"},"frequency":"monthly"}`, "request.invalid"},
		{"no amount", `{"fee_type_id":"` + typeID + `","effective_from":"2026-04-01","frequency":"monthly"}`, "fees.plan_line_invalid"},
		{"zero amount", `{"fee_type_id":"` + typeID + `","effective_from":"2026-04-01","amount":{"amount":0,"currency":"PKR"},"frequency":"monthly"}`, "fees.plan_line_invalid"},
		{"negative amount", `{"fee_type_id":"` + typeID + `","effective_from":"2026-04-01","amount":{"amount":-5,"currency":"PKR"},"frequency":"monthly"}`, "fees.plan_line_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, path, token, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400: %s", rec.Code, rec.Body)
			}
			var env struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			mustJSON(t, rec.Body.Bytes(), &env)
			want := tt.wantCode
			if want == "" {
				want = "money.currency_invalid"
			}
			if env.Error.Code != want {
				t.Fatalf("error code = %q, want %q", env.Error.Code, want)
			}
		})
	}
}

// Another school's class plan and student fees are a 404.
func TestREQ_API_FeeSetupOfAnotherTenantIs404(t *testing.T) {
	f := newFixture(t)
	a := newFeeWorld(t, f, "fee-ha")
	b := newFeeWorld(t, f, "fee-hb")
	tokenB := signInAs(t, f, b)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)

	for _, path := range []string{
		"/api/v1/fee-plans/" + a.classID,
		"/api/v1/fee-plans/" + a.classID + "/history",
		"/api/v1/students/" + a.studentID + "/fee-structure",
		"/api/v1/students/" + a.studentID + "/fee-overrides",
	} {
		if rec := do(t, h, http.MethodGet, path, tokenB, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s as the other school: %d, want 404", path, rec.Code)
		}
	}
}
