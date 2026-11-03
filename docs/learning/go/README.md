# Go learning notes

Read in this order. Each note is short and ends with a "Read next" link.

| # | Note | What you learn |
|---|---|---|
| 01 | `01-prerequisites-typescript-to-go.md` | Syntax, types, functions, errors, pointers, slices, maps, defer |
| 02 | `02-context-and-cancellation.md` | What `ctx` is and why every function takes it |
| 03 | `03-packages-modules-and-imports.md` | `go.mod`, packages, exports, `internal/`, the folder layout |
| 04 | `04-structs-methods-and-interfaces.md` | Classes vs structs, methods, implicit interfaces, injection |
| 05 | `05-runtime-lifecycle-and-goroutines.md` | Program start and exit, goroutines vs the event loop, shutdown |
| 06 | `06-http-server-routing-middleware.md` | `net/http`, `ServeMux`, handlers, middleware, `main.go` line by line |
| 07 | `07-tooling-formatting-and-linting.md` | gofmt, vet, golangci-lint, what `make check` runs |
| 08 | `08-observability-from-day-one.md` | Logs, request IDs, metrics, traces, health |

Architecture and database notes are in `../architecture/` and `../database/`.
