package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
)

// TestGetHealth exercises the route through the generated router, so it also
// proves the path in the OpenAPI spec (/healthz at the root) is the one served.
func TestGetHealth(t *testing.T) {
	handler := gen.HandlerFromMux(gen.NewStrictHandler(NewServer(), nil), http.NewServeMux())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", got)
	}
}
