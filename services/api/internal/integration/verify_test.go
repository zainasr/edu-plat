package integration

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
)

// US-IAM-003: if the same correct code is verified many times at the same instant, exactly one
// call gets a session. The consume step is one conditional UPDATE, so the database decides.
func TestUS_IAM_003_ConcurrentVerifyOfOneCodeSucceedsOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "race")

	if _, err := f.iam.RequestOTP(ctx, mobile); err != nil {
		t.Fatal(err)
	}
	code := f.sender.codeFor(mobile)

	const callers = 8
	type result struct {
		sess iam.Session
		err  error
	}
	results := make(chan result, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			s, err := f.iam.VerifyOTP(ctx, mobile, code, slug)
			results <- result{sess: s, err: err}
		})
	}
	wg.Wait()
	close(results)

	wins := 0
	for r := range results {
		if r.err == nil {
			wins++
			continue
		}
		if got := errCode(t, r.err); got != "auth.otp_invalid" {
			t.Fatalf("a losing call returned %s, want auth.otp_invalid", got)
		}
	}
	if wins != 1 {
		t.Fatalf("%d of %d concurrent verifies got a session, want exactly 1", wins, callers)
	}
}

// Every new endpoint needs a token (401) and the named permission (403). The table covers the
// whole structure and enrolment surface over real HTTP, so a handler that forgets iam.Require
// fails here.
func TestREQ_API_NewEndpointsRequireAuthAndPermission(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "authz")
	session := signIn(t, f, mobile, slug)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students), f.iam)

	// Remove every role: the session stays valid but holds no permission.
	if _, err := f.admin.Exec(ctx, `DELETE FROM role_assignments WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}

	const id = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	endpoints := []struct {
		name, method, path, body string
	}{
		{"create session", http.MethodPost, "/api/v1/sessions", `{"name":"x","starts_on":"2026-04-01","ends_on":"2027-03-31"}`},
		{"list sessions", http.MethodGet, "/api/v1/sessions", ""},
		{"make session current", http.MethodPost, "/api/v1/sessions/" + id + "/make-current", ""},
		{"create class", http.MethodPost, "/api/v1/classes", `{"name":"Class 5","grade_level":5}`},
		{"list classes", http.MethodGet, "/api/v1/classes", ""},
		{"archive class", http.MethodPost, "/api/v1/classes/" + id + "/archive", ""},
		{"create section", http.MethodPost, "/api/v1/sections", `{"class_id":"` + id + `","name":"A","shift":"morning"}`},
		{"list sections", http.MethodGet, "/api/v1/sections", ""},
		{"archive section", http.MethodPost, "/api/v1/sections/" + id + "/archive", ""},
		{"change section", http.MethodPost, "/api/v1/students/" + id + "/change-section", `{"section_id":"` + id + `","reason":"correction"}`},
		{"list enrolments", http.MethodGet, "/api/v1/students/" + id + "/enrolments", ""},
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

// US-IAM-005 over HTTP: a deactivated membership is refused on a real route, not only in the service.
func TestUS_IAM_005_DeactivatedMembershipGets401OverHTTP(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "deact-http")
	session := signIn(t, f, mobile, slug)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students), f.iam)

	if rec := do(t, h, http.MethodGet, "/api/v1/classes", session, ""); rec.Code != http.StatusOK {
		t.Fatalf("before deactivation: %d %s", rec.Code, rec.Body)
	}
	if _, err := f.admin.Exec(ctx, `UPDATE memberships SET status = 'deactivated' WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/classes", session, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after deactivation: %d, want 401: %s", rec.Code, rec.Body)
	}
}

// The new flow over HTTP: enrol, move with a reason, read the history.
func TestUS_STU_005_ChangeSectionOverHTTP(t *testing.T) {
	f := newFixture(t)
	slug, mobile, _ := newTenant(t, f, "move-http")
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students), f.iam)
	token := signIn(t, f, mobile, slug)

	rec := do(t, h, http.MethodPost, "/api/v1/sections", token,
		`{"class_id":"`+st.ClassID+`","name":"B","shift":"morning"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create section B: %d %s", rec.Code, rec.Body)
	}
	var b struct {
		ID string `json:"id"`
	}
	mustJSON(t, rec.Body.Bytes(), &b)

	student, err := f.students.Create(context.Background(), p, enrolInput(t, st.SectionID))
	if err != nil {
		t.Fatal(err)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/students/"+student.ID+"/change-section", token,
		`{"section_id":"`+b.ID+`","reason":"correction"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change section: %d %s", rec.Code, rec.Body)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/students/"+student.ID+"/enrolments", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("enrolments: %d %s", rec.Code, rec.Body)
	}
	var history struct {
		Items []struct {
			SectionName string  `json:"section_name"`
			Status      string  `json:"status"`
			Reason      *string `json:"reason"`
		} `json:"items"`
	}
	mustJSON(t, rec.Body.Bytes(), &history)
	if len(history.Items) != 2 || history.Items[0].SectionName != "B" || history.Items[0].Status != "active" ||
		history.Items[1].Status != "closed" || history.Items[1].Reason == nil || *history.Items[1].Reason != "correction" {
		t.Fatalf("history is wrong: %s", rec.Body)
	}

	// The same section again is refused with the documented code.
	rec = do(t, h, http.MethodPost, "/api/v1/students/"+student.ID+"/change-section", token,
		`{"section_id":"`+b.ID+`","reason":"again"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("same section: %d, want 400: %s", rec.Code, rec.Body)
	}
}
