package api

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/api/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/httpx"
	"github.com/zainasr/edu-plat/services/api/internal/platform/idempotency"
)

// Option changes how Handler is built.
type Option func(*handlerConfig)

type handlerConfig struct {
	idempotencyPool *pgxpool.Pool
}

// WithIdempotency makes the routes listed in idempotency.Operations honour an Idempotency-Key
// (REQ-API-010). The server always passes it; without it a key is ignored.
func WithIdempotency(pool *pgxpool.Pool) Option {
	return func(c *handlerConfig) { c.idempotencyPool = pool }
}

// Handler builds the full HTTP handler: generated routes, the one error mapping,
// token authentication, request IDs and access logs. Order matters: the request ID
// is set first so every later log line and error carries it.
func Handler(s *Server, identity *iam.Service, opts ...Option) http.Handler {
	var cfg handlerConfig
	for _, o := range opts {
		o(&cfg)
	}
	strict := gen.NewStrictHandlerWithOptions(s, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  httpx.BadRequest,
		ResponseErrorHandlerFunc: httpx.WriteError,
	})
	var middlewares []gen.MiddlewareFunc
	if cfg.idempotencyPool != nil {
		middlewares = append(middlewares, idempotency.New(cfg.idempotencyPool, func(r *http.Request) (string, string, bool) {
			p, ok := iam.From(r.Context())
			return p.TenantID, p.UserID, ok
		}))
	}
	routes := gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		Middlewares:      middlewares,
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
