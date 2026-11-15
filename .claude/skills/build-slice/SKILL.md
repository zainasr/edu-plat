---
name: build-slice
description: Build one approved backend slice end to end (contract, migration, queries, rules, service, handlers, tests, evaluation, commits). Use after /plan-slice is approved. This is the procedure for Go backend work in this repo.
argument-hint: "[approved slice, e.g. fee challans]"
disable-model-invocation: true
---
Build the approved slice: $ARGUMENTS

If there is no approved plan, stop and run `/plan-slice`. Work in this order. Each step ends with a check; do not move on while it is red. Commit at the marked points (small commits, conventional messages, see `/ship`).

**1. Contract first.** Edit `api/openapi/openapi.yaml`: paths, schemas, every status code (use `$ref` error responses, add `409` for creates that can clash), the permission in the description, Money as `{amount, currency}`. Names in `components.schemas` are one flat namespace: search before adding. Bump `info.version`. Run `make gen`.

**2. Migration (a NEW file, never an edit).** `services/api/migrations/NNNNN_name.sql` with `-- +goose Up` and `Down`, `StatementBegin/End` around any block containing semicolons. Every tenant table: `tenant_id uuid NOT NULL`, `UNIQUE (tenant_id, id)`, composite foreign keys `(tenant_id, x_id)`, an index starting with `tenant_id`, `ENABLE ROW LEVEL SECURITY` with the `NULLIF` policy (copy the DO block from `00004`). Grants: `SELECT, INSERT, UPDATE` only, never `DELETE`; history tables get no `UPDATE` either plus the `forbid_mutation` trigger. Money is `bigint` paisa. Run `make migrate`. The hooks and `TestINV_*` will refuse the common mistakes.

**3. Queries.** `internal/<module>/store/queries/<module>.sql`, one query per business need, explicit columns, named params. Add the module to `services/api/sqlc.yaml`. Run `make gen`.

**4. Rules and tests first.** Pure functions in `rules.go` (no database, no clock), and `rules_test.go` named after the spec ID (`TestBR_FEE_001_...`), table-driven. Write the tests, run them, **watch them fail**, then implement.

**5. Service.** `db.WithTenant` for every tenant query; `audit.Record` in the same transaction for every mutation; return `apperr` for client-facing failures; wrap others with `%w`. Ask other modules through methods that take the caller's `pgx.Tx`; never read their tables. Money moves only through `ledger.Post` inside the business transaction.

**6. Handlers.** In `internal/api/<module>.go`: `iam.Require(ctx, iam.Perm...)` FIRST, then the body check, then the service, then map. Add the permission constants and give the owner role the permission in `iam/principal.go`. Wire the service in `cmd/api/main.go` and `api.NewServer`, and the fixture in `integration_test.go`. Commit: contract + migration + queries + rules.

**7. Integration tests** against real Postgres (`internal/integration/`): the happy path, every refusal and its error code, tenant isolation (tenant B cannot read or change tenant A's data), concurrency for counters and uniqueness, the audit row, immutability for any history table, and HTTP-level behaviour (Money in and out, 400 shapes). `TestINV_EveryOperationRequiresAuthAndAPermission` covers 401/403 for every endpoint automatically.

**8. Evaluate.** `make check`, then `/verify`. For every NEW guarantee (a constraint, trigger, grant, authorisation rule, resolver rule) run a mutation check (`/mutation-check`); a surviving mutation means a missing test: add it. Do not claim done on a red or unrun gate.

**9. Record.** ADR (`/adr`) for a new technical choice; DEC row and spec edit in the same change if the spec moved; update `docs/engineering` docs if a convention changed; a learning note (`/learning-note`) if the owner is learning from this slice.

**10. Report** the REQ/BR/US IDs now covered and not covered, what was verified and how, what was not verified, and anything that needs a human decision. Then `/ship`.

Standards in one line: plain code, small packages, standard library first, no new dependency without an ADR, errors returned and wrapped, no globals, `ctx` first, no floats near money, never hard-delete, forward-only migrations, English UI strings externalised.
