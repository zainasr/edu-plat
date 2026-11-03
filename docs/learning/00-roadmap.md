# Go learning roadmap tied to product slices

| Stage | Concepts | Slice that teaches it |
|---|---|---|
| 1. Foundations | packages, types, structs, slices, maps, errors, testing (notes 01, 03) | Health endpoint and its test (`services/api/internal/platform/httpx`) |
| 2. Web | `net/http`, handlers, middleware, JSON, context, graceful shutdown (notes 02, 05, 06) | `cmd/api/main.go`; request ID and logging middleware |
| 3. Data | `pgx`, `sqlc`, transactions, migrations, RLS and tenant context (database notes) | Tenant + student create (REQ-STU-001) |
| 4. Design | interfaces at boundaries, small packages, injection by hand (note 04) | Students module service/store split |
| 5. Money | integer money type, table tests, invariants, append-only ledger | Challan + payment + journal (REQ-FEE) |
| 6. Concurrency | goroutines, channels, `errgroup`, worker pools, queues (note 05) | PDF and notification jobs |
| 7. Production | slog, metrics, tracing, config, idempotency, retries (note 08) | Online payment webhook (REQ-FEE-031) |
| 8. Quality | race detector, fuzzing, benchmarks, profiling | Import module (5,000 rows) |

Rule: one concept per slice, read the code, write the note, do the exercise.
