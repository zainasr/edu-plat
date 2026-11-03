# 08. Observability from day one: logs, metrics, traces and health

**Why from day one:** you cannot fix what you cannot see. A fee payment that fails at 2 pm on the 10th of the month must be explainable from data, not guesses. Instrumenting later is much harder, because every handler must change.

## 1. The three signals, plus health

| Signal | Question it answers | Tool in our plan | Status |
|---|---|---|---|
| **Logs** | "What happened in this request?" | `log/slog`, JSON output | In `main.go` now |
| **Metrics** | "How many, how fast, how often?" (rates, percentiles) | Prometheus-format `/metrics` | Add with the first module |
| **Traces** | "Where did the time go across layers and services?" | OpenTelemetry | Add when the first external call (aggregator) lands |
| **Health** | "Is this instance able to serve?" | `/healthz`, `/readyz` | `/healthz` now |

## 2. Structured logging with `slog`

```go
log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
log.Info("payment recorded", "receipt_id", id, "tenant_id", tenantID, "amount_paisa", 1200000)
```
Output (one JSON line):
```json
{"time":"2026-11-01T10:12:01Z","level":"INFO","msg":"payment recorded","receipt_id":"R-2026-000123","tenant_id":"...","amount_paisa":1200000}
```
- **Key-value pairs** instead of string concatenation. Searchable and parseable.
- Never log secrets, OTPs, full mobile numbers or full CNICs (already in the Go rules).
- Log at the boundary and on failures. Do not log every line of business logic.

Node equivalent: `pino` with JSON output.

## 3. Request IDs: connect one request across all logs

A middleware creates a request ID, stores it in the context, and every log line includes it. Then one query shows the whole story of one request.

```go
func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := r.Header.Get("X-Request-ID")
        if id == "" {
            id = uuid.NewString()                  // a UUID library; or crypto/rand
        }
        w.Header().Set("X-Request-ID", id)         // echoed back so the client can report it
        next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), id)))
    })
}
```
The error format in the API spec already includes `request_id`, so support can find the logs from a customer's screenshot.

## 4. Metrics: count and time things

Typical metrics for this product:
- `http_requests_total{route, status}` counter
- `http_request_duration_seconds{route}` histogram (shows p50 and p95 latency)
- `jobs_pending` gauge (queue depth, see ADR-0005)
- `payments_recorded_total` counter, to notice a sudden drop

Expose them on a separate port or path (for example `/metrics`) that the public internet cannot reach.

Node equivalent: `prom-client`.

## 5. Traces: the path through the layers

A trace is a tree of **spans**: request → handler → service → database query → aggregator call. Each span has a start, end and attributes. When the payment webhook is slow, the trace shows whether it was the database or the aggregator.

Node equivalent: OpenTelemetry JS SDK. The OpenTelemetry Go SDK works the same way.

## 6. Health checks and graceful shutdown together
- `/healthz`: process up. Cheap, no dependencies.
- `/readyz`: database reachable, migrations applied. Used by the load balancer and by blue-green deploys before switching traffic.
- On SIGTERM, flip readiness to "not ready" first, wait a few seconds for the load balancer to notice, then call `Shutdown`. This avoids dropping requests during deploys.

## 7. The minimum we add with the first module
1. A request ID middleware.
2. A logging middleware with method, route, status, duration and request ID.
3. A `/metrics` endpoint with the HTTP counter and histogram.
4. A readiness endpoint that pings the database.

We keep this small on purpose. Dashboards and alerts are built later from the metrics that already exist.

## Read next
- The three pillars, explained by Google SRE book chapter on monitoring: https://sre.google/sre-book/monitoring-distributed-systems/
- OpenTelemetry Go getting started: https://opentelemetry.io/docs/languages/go/getting-started/
- Next: `docs/learning/architecture/01-three-tier-and-request-flow.md`
