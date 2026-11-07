package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetHealth goes through the full handler: generated router, middleware, error mapping.
// It proves /healthz at the root is the served path (spec servers: /), and that /api/v1/healthz
// is not a route (the spec has no such path).
func TestGetHealth(t *testing.T) {
	handler := Handler(NewServer(nil, nil, nil, nil), nil)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", got)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("every response must carry X-Request-ID")
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/api/v1/healthz status = %d, want 404", rec.Code)
	}
}

// TestErrorEnvelope checks the REQ-API-011 shape for a request the router rejects before
// any handler runs, and that a malformed bearer token is a 401 in the envelope.
func TestErrorEnvelope(t *testing.T) {
	handler := Handler(NewServer(nil, nil, nil, nil), nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", "Basic abc")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"error":`, `"code":"auth.token_invalid"`, `"request_id":"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body %s is missing %s", body, want)
		}
	}
}
