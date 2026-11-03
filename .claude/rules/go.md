---
paths:
  - "services/api/**/*.go"
---
# Go backend rules
- Standard library `net/http` (Go 1.22+ method+path patterns). No web framework.
- Layering per module: handler (HTTP only) -> service (rules, transactions) -> store (sqlc). Handlers never contain business rules; stores never contain HTTP.
- Return errors; wrap with `fmt.Errorf("doing x: %w", err)`. Use sentinel/typed errors mapped to the API error format in one place (`httpx`). No `panic` for control flow.
- `context.Context` is the first parameter of anything that does I/O. Tenant id and actor travel in the context, set by middleware, never read from request bodies.
- Table-driven tests with `t.Run`; name tests after the spec ID (e.g. `TestBR_FEE_002_ReceiptNumbersHaveNoGaps`). Integration tests use a real Postgres, not mocks of SQL.
- Money: `int64` minor units in a small `Money` type. No float64 near money.
- Log with `log/slog` (structured). Never log secrets, OTPs or full mobile numbers.
- Goroutines must have an owner and a way to stop (context). Prefer simple code over clever concurrency.
- Idempotency: money-moving POSTs honour `Idempotency-Key` (REQ-API-010).
