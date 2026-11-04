package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/students"
)

// do sends one request through the full handler, as curl would.
func do(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestUS_IAM_004_TokenCreatesAndReadsStudentOverHTTP walks the terminal workflow of
// US-IAM-004 through the real handler: sign in, create a personal access token, enrol
// a student with the token, read it back, and check the refusals.
func TestUS_IAM_004_TokenCreatesAndReadsStudentOverHTTP(t *testing.T) {
	f := newFixture(t)
	slug, mobile, tenantID := newTenant(t, f, "http")
	otherSlug, otherMobile, _ := newTenant(t, f, "other")

	h := api.Handler(api.NewServer(f.iam, students.NewService(f.app)), f.iam)

	// 1. Request and verify an OTP over HTTP.
	if rec := do(t, h, http.MethodPost, "/api/v1/auth/otp/request", "", `{"mobile":"`+mobile+`"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("otp request: %d %s", rec.Code, rec.Body)
	}
	code := f.sender.codeFor(mobile)
	rec := do(t, h, http.MethodPost, "/api/v1/auth/otp/verify", "",
		`{"mobile":"`+mobile+`","code":"`+code+`","tenant_slug":"`+slug+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("otp verify: %d %s", rec.Code, rec.Body)
	}
	var session struct {
		Token  string `json:"token"`
		Tenant struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"tenant"`
	}
	mustJSON(t, rec.Body.Bytes(), &session)
	if session.Tenant.ID != tenantID {
		t.Fatalf("session is for tenant %s, want %s", session.Tenant.ID, tenantID)
	}

	// 2. Create a personal access token with the session.
	rec = do(t, h, http.MethodPost, "/api/v1/tokens", session.Token, `{"name":"nightly-import"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body)
	}
	var pat struct {
		Token string `json:"token"`
	}
	mustJSON(t, rec.Body.Bytes(), &pat)

	// 3. Enrol a student with the token.
	body := `{
		"full_name": "Ali Khan",
		"father_name": "Imran Khan",
		"gender": "male",
		"date_of_birth": "2018-04-12",
		"guardians": [{"full_name": "Kamran Khan", "mobile": "` + newMobile(t) + `",
		               "relationship": "uncle", "primary": true}]
	}`
	rec = do(t, h, http.MethodPost, "/api/v1/students", pat.Token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create student: %d %s", rec.Code, rec.Body)
	}
	var created struct {
		ID             string `json:"id"`
		RegistrationNo string `json:"registration_no"`
		Family         struct {
			Number string `json:"number"`
		} `json:"family"`
		Guardians []struct {
			Primary  bool `json:"primary"`
			FeePayer bool `json:"fee_payer"`
		} `json:"guardians"`
	}
	mustJSON(t, rec.Body.Bytes(), &created)
	if created.RegistrationNo == "" || created.Family.Number == "" {
		t.Fatalf("registration and family numbers are missing: %s", rec.Body)
	}
	if len(created.Guardians) != 1 || !created.Guardians[0].Primary || !created.Guardians[0].FeePayer {
		t.Fatalf("primary guardian should be the fee payer by default: %s", rec.Body)
	}

	// 4. Read it back.
	if rec := do(t, h, http.MethodGet, "/api/v1/students/"+created.ID, pat.Token, ""); rec.Code != http.StatusOK {
		t.Fatalf("read student: %d %s", rec.Code, rec.Body)
	}

	// 5. Refusals.
	t.Run("no token is 401", func(t *testing.T) {
		if rec := do(t, h, http.MethodGet, "/api/v1/students/"+created.ID, "", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", rec.Code)
		}
	})
	t.Run("another tenant's token gets 404, not 403", func(t *testing.T) {
		otherSessionToken := signIn(t, f, otherMobile, otherSlug)
		rec := do(t, h, http.MethodGet, "/api/v1/students/"+created.ID, otherSessionToken, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status %d, want 404 (no existence leak)", rec.Code)
		}
	})
	t.Run("a bad bearer value is 401 on every route", func(t *testing.T) {
		if rec := do(t, h, http.MethodGet, "/healthz", "not-a-token", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", rec.Code)
		}
	})
	t.Run("the list shows the student", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/students?limit=5", pat.Token, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list: %d %s", rec.Code, rec.Body)
		}
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		mustJSON(t, rec.Body.Bytes(), &page)
		found := false
		for _, it := range page.Items {
			found = found || it.ID == created.ID
		}
		if !found {
			t.Fatal("the newly created student is not in the first page")
		}
	})
}

// TestUS_IAM_005_DeactivatedMembershipEndsAccess checks that deactivating a membership ends
// access at once, even for a live session (US-IAM-005).
func TestUS_IAM_005_DeactivatedMembershipEndsAccess(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "revoke")
	session := signIn(t, f, mobile, slug)
	if _, err := f.iam.Authenticate(ctx, session); err != nil {
		t.Fatalf("session should work before deactivation: %v", err)
	}
	if _, err := f.admin.Exec(ctx, `UPDATE memberships SET status = 'deactivated' WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := f.iam.Authenticate(ctx, session); err == nil {
		t.Fatal("a deactivated membership must end access immediately (US-IAM-005)")
	}
}

func mustJSON(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
}
