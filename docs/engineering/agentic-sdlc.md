# Agentic SDLC: tooling inventory

What exists to keep AI-assisted development safe, and how to extend it. The procedure itself is in `development-workflow.md`.

## Principle
One definition of "good" (`make evaluate`) shared by the builder, the independent agents, the Stop hook and CI. Rules that can be checked by a machine are enforced by hooks and tests; judgement is left to agents and the owner.

## Skills (`.claude/skills/`)
| Skill | Use |
|---|---|
| `dev-context` | Load context in the right order at the start of any task |
| `plan-slice` | One-screen plan; stops for the owner's approval |
| `build-slice` | Contract → migration → queries → tests first → code |
| `verify` | `make evaluate` + evaluator, spec-auditor, reviewer agents + mutation checks; prints a verdict |
| `mutation-check` | Break a rule on purpose and watch a test fail |
| `ship` | Small commits, approved push, watch CI |
| `ask-human` | When and how to stop for the owner |
| `frontend-slice` | The same loop for `apps/web` |
| `adr`, `learning-note` | Record a decision; write a learning note |
| `spec-slice` | Retired pointer to the four above |

## Agents (`.claude/agents/`, read-only)
`evaluator` (runs the gates and checks what they cannot see), `spec-auditor` (requirement IDs against tests), `reviewer` (tenant isolation, money, idempotency, audit).

## Hooks (`.claude/hooks/`, registered in `.claude/settings.json`)
| Hook | Event | Effect |
|---|---|---|
| `session-start.sh` | SessionStart | Prints branch, database, migration version, CI |
| `guard-paths.sh` | PreToolUse Edit/Write | Blocks generated code and applied migrations |
| `guard-money.sh` | PreToolUse Edit/Write | Blocks `float` in money code |
| `guard-migration.sh` | PreToolUse Edit/Write | New migrations need RLS; no DROP/TRUNCATE/GRANT DELETE |
| `guard-secrets.sh` | PreToolUse Edit/Write | Blocks secrets and `.env` writes (uses `scripts/secret-scan.sh`) |
| `guard-git.sh` | PreToolUse Bash | Blocks force push, `--no-verify`, destructive git; `git push` runs `make check` first |
| `format-go.sh` | PostToolUse | gofmt on edited Go files |
| `stop-check.sh` | Stop | The turn cannot end with unchecked code changes (compares `scripts/fingerprint.sh` with the stamp from `make check`) |

## Scripts (`scripts/`)
`evaluate.sh` (the 13 gates), `mutate.sh`, `mutate-db.sh`, `spec-coverage.sh`, `fingerprint.sh`, `stamp.sh`, `secret-scan.sh`, `test-hooks.sh`.

## Makefile targets
`check`, `evaluate`, `test-hooks`, `spec-coverage MOD=`, plus the existing `test`, `lint`, `gen`, `run`, `db-up`, `db-roles`, `migrate`.

## CI
`.github/workflows/ci.yml` runs `make evaluate` against a Postgres service. Same gates as local.

## How the tooling is tested
`make test-hooks` (`scripts/test-hooks.sh`) feeds each hook real and false-positive inputs (including a sandbox git repo for the Stop hook and a sandbox Makefile for the push gate). Mutation scripts are exercised against known rules. Run `make test-hooks` after any change to `.claude/` or `scripts/`; `make evaluate` includes it.

## Adding or changing a hook, skill or agent
1. Write the failing case in `scripts/test-hooks.sh` first (a block case and a must-not-block case).
2. Implement it; keep it a small shell script that reads the tool input JSON from stdin and exits 2 with a message to block.
3. Register it in `.claude/settings.json`; make it executable.
4. `make test-hooks`; update this file.
5. Hooks, skills and agents load at session start: restart the session to use them.

## Known limits
- Hooks guard the common paths, not a determined bypass; reviewers and CI are the second line.
- Agents are judges with tools, not proof; their findings need the evidence they cite.
- Mutation checks are manual per guarantee; the invariant tests cover the system-wide rules automatically.
- The `evaluator`, `spec-auditor` and the new skills were written and their underlying scripts tested, but they were first loaded by Claude Code only after the session that created them. If one misbehaves, fix its file and note it.

## Decision protocol
See `development-workflow.md` section 8 and `.claude/skills/ask-human/SKILL.md`.
