# Education Management Platform

Multi-tenant SaaS for Pakistani schools (web + mobile). The locked product spec is the source of truth: `docs/spec/README.md` (Baseline v1.0). Decisions are in `docs/spec/01-decisions/` and `docs/adr/`.

## Commands (run these, do not invent others)
- `make check`   lint + vet + all tests (run before saying work is done)
- `make test`    Go tests; one test: `cd services/api && go test ./internal/fees -run TestName`
- `make lint`    gofmt check + go vet
- `make run`     start the API on :8080
- `make db-up`   start local Postgres (Docker)
- `make gen`     regenerate code from OpenAPI and SQL (sqlc); never hand-edit generated files

## Working rules
- Before coding a feature, read its module in `docs/spec/03-modules/` and satisfy its REQ/BR IDs and Given/When/Then criteria. Put the IDs in test names or comments.
- The OpenAPI file `api/openapi/openapi.yaml` is edited FIRST; code follows it. Every capability is an endpoint (DEC-004).
- Go: standard library first; add a dependency only with a short ADR. Plain code, small packages, errors returned and wrapped with `%w`, no global state, `context.Context` first parameter.
- Every tenant table has `tenant_id` and an RLS policy; every query runs in a tenant-scoped transaction. A missing tenant filter is a bug.
- Money is integer minor units (PKR paisa). Never float. Financial actions post balanced journal entries; receipts and journals are append-only, corrected by reversal. Never hard-delete business records (DEC-011).
- Migrations are forward-only and backward-compatible. Never edit an applied migration; add a new one.
- Spec changes after lock: edit the spec in the same PR and add a DEC row (see `docs/spec/README.md`, "Change rule after lock").
- New significant technical choice: write an ADR (`/adr`). Each finished slice adds a note in `docs/learning/` (`/learning-note`).
- UI text is English and externalised; no hard-coded product name (DEC-015, DEC-017).

## Where things live
Stack-specific rules load automatically from `.claude/rules/` when you touch matching files. Layout and conventions: `docs/engineering/repo-layout.md`.
