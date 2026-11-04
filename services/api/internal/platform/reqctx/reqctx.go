// Package reqctx carries per-request values (request ID) through context.Context.
// Services read it for audit entries without importing net/http.
package reqctx

import "context"

type requestIDKey struct{}

// WithRequestID returns a context that carries the request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request ID, or "" when none was set (for example in a CLI).
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
