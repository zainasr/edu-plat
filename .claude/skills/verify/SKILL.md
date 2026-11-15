---
name: verify
description: Independently evaluate finished work before it is committed or called done. Runs make evaluate, spawns the evaluator, spec-auditor and reviewer agents, runs mutation checks on new guarantees, and returns a pass/fail report with evidence. Use after building or changing code, schema, the contract or tooling.
argument-hint: "[what changed, e.g. fee challans slice]"
allowed-tools: Read Glob Grep Bash(make *) Bash(scripts/*) Bash(git *) Bash(go test *) Agent
---
Evaluate: $ARGUMENTS

You build; others judge. Do not mark your own work as passing from memory.

**1. Run the evaluation.** `make evaluate`. It forces uncached tests, regenerates code, builds the whole schema in a scratch database, runs the invariants (RLS everywhere, no DELETE grants, money is bigint, history tables append-only, every endpoint needs auth and a permission), and scans for secrets, floats in money code and hard deletes. Any FAIL is a stop: read the log it prints, fix the cause, run again. Never edit a gate or a test to make it pass unless the gate itself is wrong, and then say so.

**2. Run the independent agents in parallel** (they are read-only):
- `evaluator`: re-runs `make evaluate` on a clean read and checks the changed files against the checklist in `docs/engineering/development-workflow.md`.
- `spec-auditor`: `scripts/spec-coverage.sh` for the module, then reads the spec module and reports each REQ/BR/US as covered, partly covered or missing, with the test name.
- `reviewer`: tenant isolation, money, idempotency, permissions, contract match.
Give each the slice name and the commit range (`git diff main...HEAD` or the uncommitted diff). If an agent type is not available in this session (agents load at session start), say so and do that review yourself, step by step.

**3. Mutation checks for every new guarantee** (`/mutation-check`). At least one KILLED result per new rule; fix every SURVIVED one with a test.

**4. If the tooling changed** (`.claude/`, `scripts/`, the Makefile, CI): `make test-hooks` must pass, and add a test case for any new hook behaviour.

**5. Report** in this shape and nothing longer:
```
VERDICT: ship | fix first | rethink
Gates: <n> of <n> passed   (list any FAIL)
Spec: covered <IDs>; missing <IDs>
Mutations: <n> killed, <n> survived (list)
Not verified: <what you could not or did not check, and why>
Needs a human: <decisions, or none>
```
Be honest about "Not verified": a short true list beats a long confident one.
