---
name: dev-context
description: Load the right context before changing anything in this repo, in the right order. Use at the start of every task, in a fresh session, and before planning or building a slice.
argument-hint: "[area, e.g. fees, students, ledger, frontend]"
allowed-tools: Read Glob Grep Bash(git *) Bash(ls *) Bash(scripts/*) Bash(make spec-coverage *)
---
Gather context for: $ARGUMENTS

Do these in order. Read only what the task needs; do not read all of `docs/spec/03-modules/` or all of `docs/learning/`.

**1. The real state (2 minutes).** The session-start hook already printed branch, database and CI state. Add:
`git log --oneline -15` · `git status --short` · `ls services/api/migrations | tail -3` · `ls services/api/internal`.
If the database is down: `make db-up && make db-roles && make migrate`. If the tree has changes that are not yours, say so and leave them alone.

**2. The rules (always).** `CLAUDE.md` (loaded) → `docs/engineering/development-workflow.md` (the procedure and standards) → the `.claude/rules/*.md` files for the file types you will touch (they load by path).

**3. Decisions already made.** `docs/adr/README.md` (one line per ADR; read the ones that touch your area) and `docs/spec/01-decisions/01-decision-log.md` (DEC-001 onward). A decision here is settled: do not re-argue it, and do not contradict it without asking (`/ask-human`).

**4. The requirement.** The module file in `docs/spec/03-modules/NN-*.md` for the slice. Write down the REQ, BR and US IDs and the Given/When/Then lines. Then the cross-cutting files that apply:
`02-cross-cutting/04-api-conventions.md` (every endpoint), `02-identity-roles-permissions.md` (permission names), `03-data-model.md`, `01-organisation-and-structure.md` (school structure).
Run `make spec-coverage MOD=<code>` to see which IDs of the module are already tested.

**5. What exists.** `api/openapi/openapi.yaml` (search for the paths you will touch), the migration list, and one neighbouring module as the pattern to copy:
- the simplest module: `services/api/internal/org/` (rules, service, store, handlers in `internal/api/structure.go`)
- versioned data and a pure resolver: `internal/fees/`
- money and the ledger: `internal/ledger/` and `internal/platform/money/`
- identity and tokens: `internal/iam/`
Read its `rules.go`, `service.go`, `store/queries/*.sql` and the matching handler file.

**6. How it is tested.** `services/api/internal/integration/integration_test.go` (the fixture), `helpers_test.go` (`principalFor`, `newStructure`, `errCode`), `invariants_test.go` (rules that hold for every slice), and one existing test file for the same kind of work.

**7. Report back in six lines or fewer:** what the task is, which IDs it covers, which decisions apply, which module you will copy, what you do not yet know, and what you will ask the human. Do not start coding from this skill.

If the work is a frontend task, also read `docs/engineering/frontend.md` and `docs/adr/0016-web-stack.md`, then use `/frontend-slice`.
