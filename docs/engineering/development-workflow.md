# Development workflow

How to change this repository, in order. `CLAUDE.md` is the short version; this is the full one. It applies to humans and agents alike.

## 1. The loop

`/dev-context` → `/plan-slice` → **owner approves** → `/build-slice` → `/verify` → `/ship` (**owner approves the push**)

A slice is one vertical piece of a spec module: contract, migration, queries, rules, service, handlers, tests. Small fixes (a typo, a one-line bug with a failing test) skip the plan but not `/verify`.

## 2. Reading order (gather context)
1. `git log --oneline -10`, `git status`, and the SessionStart output (branch, database, migration version, CI).
2. `CLAUDE.md`, this file, `.claude/rules/*` for the stack you touch.
3. `docs/adr/README.md` and `docs/spec/01-decisions/01-decision-log.md`.
4. The module in `docs/spec/03-modules/`, plus the cross-cutting files: `04-api-conventions`, `02-identity-roles-permissions`, `03-data-model`, `01-organisation-and-structure`. Run `make spec-coverage MOD=<code>` to see which IDs are tested.
5. `api/openapi/openapi.yaml`, `services/api/migrations/`, and one neighbouring module to copy (`org`, `fees`, `ledger`, `iam`).
6. Test fixtures: `integration_test.go`, `helpers_test.go`, `invariants_test.go`.
7. Frontend work also reads `docs/engineering/frontend.md` and `docs/adr/0016-web-stack.md`.

## 3. Standards
- **Simplicity.** The smallest design that meets the requirement. No abstraction before the second use, no framework, no speculative options. Delete code you do not need.
- **Go.** Standard library first; a dependency needs an ADR. Small packages, `context.Context` first, errors wrapped with `%w`, no global state. Handler (HTTP only, `iam.Require` first) → service (rules, transaction, audit, `apperr`) → store (sqlc).
- **SQL.** Forward-only migrations; never edit an applied one. Every tenant table: `tenant_id`, tenant-leading index, composite `(tenant_id, id)` foreign keys, RLS with the `NULLIF` policy, no `DELETE` for `app_rt`. History tables are append-only.
- **API.** Edit `openapi.yaml` first, then `make gen`. Errors use `{error:{code,message,details,request_id}}` with stable `module.reason` codes. Money is `{amount, currency}` with an integer amount.
- **Money.** `money.Money` (paisa). Never float. Post balanced entries with `ledger.Post` in the business transaction; correct by reversal.
- **Audit.** Every mutation writes an audit entry in the same transaction.
- **Never hard-delete** business records (DEC-011).
- **UI text** is English, externalised, no hard-coded product name.

## 4. Backend slice recipe
1. Plan (`/plan-slice`): goal, requirement IDs, design, riskiest part, decisions needed. Stop.
2. Contract: edit `openapi.yaml`; `make gen`.
3. Migration: new numbered file, with Down; run `make migrate`.
4. Queries: `queries/*.sql`; `make gen`.
5. **Tests first**: failing tests named with the spec ID (`TestREQ_…`, `TestBR_…`, `TestUS_…`).
6. Rules → service → handlers until green.
7. `make check`, then `/verify`.

## 5. Testing pyramid
- **Unit** (pure rules, no database): boundaries and failure cases, not only the happy path.
- **Integration** (real Postgres, `app_rt` role): the service and database together; RLS, constraints, concurrency.
- **HTTP** (through the mux): 401 without a token, 403 without a permission, error envelope, contract shape.
- **Invariants** (`TestINV_*`): rules about the whole system, written once, applied to every table and endpoint, including ones added later. A new table or endpoint that breaks one fails automatically.
- **Mutation checks**: for each new guarantee, break the one line that carries it on purpose and see a test fail. KILLED is required; a SURVIVED mutation means a missing test. Use `scripts/mutate.sh` (code) and `scripts/mutate-db.sh` (database); examples are in `.claude/skills/mutation-check/SKILL.md`.
- Use `go test -count=1` when checking a result; the cache once hid a stopped database.

## 6. `make check` versus `make evaluate`
- `make check`: lint + vet + tests. Fast, run while building. It writes a stamp that lets the Stop hook know the current code was checked.
- `make evaluate`: the independent gate (about 30 s): uncached tests, invariants, regenerated code must match committed code, migrations apply to a scratch database from zero, hook tests, secret scan. CI runs the same command.
- `/verify` adds the `evaluator`, `spec-auditor` and `reviewer` agents and mutation checks. The builder never grades its own work.

## 7. Git and CI
- Small commits, conventional messages (`feat(fees): …`, `fix(iam): …`, `test:`, `docs:`, `chore:`). Contract, migration and generated code in the same commit.
- Never `--no-verify`, force push, or destructive git commands (hooks block them).
- Push only after the owner's yes for that push. The push hook runs `make check` first. Then watch CI: `gh run list`, `gh run watch`.
- Spec change after lock: edit the spec in the same PR and add a DEC row. New significant technical choice: an ADR (`/adr`).

## 8. Human in the loop (`/ask-human`)
Stop and ask before deciding anything about money rules, security, permissions, a new public endpoint, tokens or cookies; when spec, code and the owner's words conflict (quote both, recommend one); before changing scope, adding a dependency, breaking the API, or anything destructive; before every push. Do not ask about naming, layout or anything a rule or ADR already settles: state the assumption and go on. If blocked, finish the independent work and begin the final message with the exact question.

## 9. Checklists
**New table:** `tenant_id` · tenant-leading index · composite FKs · RLS `NULLIF` policy · grants without DELETE · append-only trigger if history · Down section · `TestINV_*` still green · scratch migration gate green.
**New endpoint:** in `openapi.yaml` first · `iam.Require` with a named permission from the catalogue · 401/403 covered (the invariant test does it) · audit entry on mutations · error codes in the contract · public endpoints are not allowed without the owner's decision · add a row to `TestREQ_IAM_005_BuiltInRolesHoldTheirPermissions` (`internal/integration/roles_test.go`) saying which built-in roles may call it.
**Money action:** one table row per fact, balances derived not stored on immutable records; a database trigger at commit for any sum that must add up (see ADR-0014, ADR-0020) · a POST that creates or moves money declares the `Idempotency-Key` parameter and is added to `idempotency.Operations` (ADR-0018; a test keeps them equal) · `money.Money` only · `ledger.Post` in the same transaction · balanced · idempotent where retried · reversal instead of edit · a mutation check on the balance rule.
**New dependency:** ask the owner · short ADR · pinned version.
**Hook or skill change:** add a case to `scripts/test-hooks.sh` · `make test-hooks` · update `docs/engineering/agentic-sdlc.md`.

## 10. Troubleshooting
- Tests fail with connection refused: the Postgres container stopped. `make db-up && make db-roles && make migrate`.
- `make gen` produces a diff in CI: you edited a source and did not commit the regenerated files.
- A hook blocks you: read its message; it names the rule. Do not look for a way around it; ask the owner if the rule seems wrong.
- Editor diagnostics disagree with the compiler: trust `go build` and `go vet`.
- zsh expands `?` and `*`: quote URLs and patterns.
