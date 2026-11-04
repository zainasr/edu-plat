package api

import (
	"net/http"
	"strings"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/httpx"
)

// Handler builds the full HTTP handler: generated routes, the one error mapping,
// token authentication, request IDs and access logs. Order matters: the request ID
// is set first so every later log line and error carries it.
func Handler(s *Server, identity *iam.Service) http.Handler {
	strict := gen.NewStrictHandlerWithOptions(s, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  httpx.BadRequest,
		ResponseErrorHandlerFunc: httpx.WriteError,
	})
	routes := gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		ErrorHandlerFunc: httpx.BadRequest,
	})
	return httpx.RequestID(httpx.AccessLog(authenticate(identity, routes)))
}

// authenticate resolves a bearer token into a principal. No header means anonymous: public
// routes still work, and protected handlers reject with 401 through iam.Require. A malformed
// or invalid token is rejected here, on every route.
func authenticate(identity *iam.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			next.ServeHTTP(w, r)
			return
		}
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			httpx.WriteError(w, r, apperr.Unauthenticated("auth.token_invalid", "The token is not valid."))
			return
		}
		p, err := identity.Authenticate(r.Context(), token)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(iam.WithPrincipal(r.Context(), p)))
	})
}
