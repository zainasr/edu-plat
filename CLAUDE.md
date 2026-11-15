# Education Management Platform

Multi-tenant SaaS for Pakistani schools (web + mobile). The locked product spec is the source of truth: `docs/spec/README.md` (Baseline v1.0). Decisions are in `docs/spec/01-decisions/` and `docs/adr/`.

## How to work here (read this first)
You work as a staff engineer with a human product owner. The loop is fixed:

`/dev-context` → `/plan-slice` → **owner approves** → `/build-slice` → `/verify` → `/ship` (owner approves the push)

- **Start every task** with `/dev-context` (reading order, standards, the module to copy). The session-start hook prints the real state: branch, database, migrations, CI.
- **Plan before code** for anything beyond a small fix. A plan is one screen: goal, requirement IDs, design, riskiest part, decisions needed. Then stop.
- **You build, others judge.** `/verify` runs `make evaluate` and the `evaluator`, `spec-auditor` and `reviewer` agents. Do not mark your own work as passing from memory.
- **Prove tests are real.** Each new guarantee needs a mutation check that is KILLED (`/mutation-check`). A passing test you never saw fail proves nothing.
- Full procedure, standards and checklists: `docs/engineering/development-workflow.md`. Tooling inventory and how to extend it: `docs/engineering/agentic-sdlc.md`. Web: `docs/engineering/frontend.md`.

## Ask the human (`/ask-human`) before you
- decide anything about money rules, security, permissions, a new public endpoint, token or cookie handling;
- resolve a conflict between the spec, the code and what the owner said (quote both sides, recommend one);
- change scope, add a dependency, break the API, or run anything destructive or hard to undo (drop, delete, reset, force push);
- push to the remote (a yes for an earlier push does not cover this one).
Do not ask about naming, layout or anything this file, a rule or an ADR already settles: state the assumption and continue. When blocked, finish the independent work and open your final message with the exact question.

## Commands (run these, do not invent others)
- `make check`      lint + vet + all tests. Run before saying work is done. Needs the database (`make db-up`).
- `make evaluate`   the full independent evaluation: uncached tests, invariants, fresh generated code, scratch-database migrations, hook tests, secret scan. Run via `/verify`.
- `make test`       Go tests; one test: `cd services/api && go test -count=1 ./internal/fees -run TestName`
- `make lint`       gofmt check + go vet + golangci-lint
- `make run`        start the API on :8080 (needs `APP_ENV=dev`, `DATABASE_URL`, `AUTH_SECRET`)
- `make db-up` / `make db-roles` / `make migrate`   start Postgres, create the `app_rt` role, apply migrations
- `make gen`        regenerate code from OpenAPI and SQL (sqlc); never hand-edit generated files
- `make test-hooks` test the Claude hooks; run after any change to `.claude/` or `scripts/`
- `make spec-coverage MOD=FEE`   which requirement IDs of a module are tested
- `scripts/mutate.sh` and `scripts/mutate-db.sh`   mutation checks (see `/mutation-check`)

## Working rules
- Before coding a feature, read its module in `docs/spec/03-modules/` and satisfy its REQ/BR IDs and Given/When/Then criteria. Put the IDs in test names or comments.
- The OpenAPI file `api/openapi/openapi.yaml` is edited FIRST; code follows it. Every capability is an endpoint (DEC-004). Money crosses the API as `{amount, currency}` with an integer amount.
- Go: standard library first; add a dependency only with a short ADR. Plain code, small packages, errors returned and wrapped with `%w`, no global state, `context.Context` first parameter. Layers: handler → service → store. Handlers translate, services decide, stores run SQL.
- Every tenant table has `tenant_id`, a tenant-leading index, composite tenant foreign keys and an RLS policy (the `NULLIF` form); every query runs in `db.WithTenant`. A missing tenant filter is a bug. Modules ask each other questions through methods that take the caller's `pgx.Tx` (ADR-0013); a module never reads another module's tables.
- Money is integer minor units (PKR paisa, `money.Money`). Never float. Financial actions post balanced journal entries through `ledger.Post` in the same transaction; receipts and journals are append-only, corrected by reversal. Never hard-delete business records (DEC-011): archive, deactivate, revoke or reverse. The API role has no `DELETE`.
- Every mutation writes an audit entry in the same transaction. Every handler calls `iam.Require` with a named permission first.
- Migrations are forward-only and backward-compatible. Never edit an applied migration; add a new one.
- Simplicity: the smallest design that meets the requirement. No abstraction before the second use, no framework, no clever concurrency, no speculative options. Delete code you do not need.
- Spec changes after lock: edit the spec in the same PR and add a DEC row (see `docs/spec/README.md`, "Change rule after lock").
- New significant technical choice: write an ADR (`/adr`). A finished slice may add a note in `docs/learning/` (`/learning-note`).
- UI text is English and externalised; no hard-coded product name (DEC-015, DEC-017).

## Enforced by hooks and tests (you cannot skip these, so do not try)
Hooks (`.claude/hooks/`): no edits to generated code or applied migrations; no `float` in money code; new migrations must have RLS on tenant tables, no `DROP`/`TRUNCATE`, no `GRANT DELETE`; no secrets or `.env` writes; no force push, `--no-verify` or destructive git commands; `git push` runs `make check` first; the turn cannot end with unchecked code changes.
Tests (`TestINV_*`): RLS and a tenant index on every tenant table, tenant composite foreign keys, no `DELETE`/`TRUNCATE` for the API role, money columns are `bigint`, history tables append-only, every endpoint returns 401 without a token and 403 without a permission, and the public endpoints are exactly health, request OTP and verify OTP.

## Definition of done
`make check` green · `make evaluate` green · every new guarantee has a KILLED mutation · requirement IDs listed as covered or not · contract, migration and generated code committed together · ADR or DEC row if a decision was made · small commits with conventional messages · pushed only after the owner's yes, and CI green.

## Where things live
Stack-specific rules load automatically from `.claude/rules/` when you touch matching files. Layout and conventions: `docs/engineering/repo-layout.md`. Skills: `.claude/skills/`. Agents: `.claude/agents/`. Hooks: `.claude/hooks/` (registered in `.claude/settings.json`). Scripts: `scripts/`.
