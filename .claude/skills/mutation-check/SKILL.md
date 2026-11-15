---
name: mutation-check
description: Prove a test really protects a rule by breaking the rule on purpose and watching a test fail. Use for every new guarantee (constraint, trigger, grant, auth check, resolver or money rule) and whenever a test passed on its first run.
argument-hint: "[rule to check, e.g. a later fee version must not change earlier months]"
allowed-tools: Read Glob Grep Bash(scripts/*) Bash(go test *) Bash(git *)
---
Mutation-check: $ARGUMENTS

A test you have never seen fail may not test anything. Two scripts do this safely and restore everything.

**Code rule** (Go, OpenAPI): `scripts/mutate.sh FILE 'exact text' 'replacement' PACKAGE [TEST_REGEX]`
Example: `scripts/mutate.sh services/api/internal/fees/rules.go 'if v.EffectiveFrom.After(month) {' 'if false && v.EffectiveFrom.After(month) {' ./internal/fees/ 'TestBR_FEE_001'`
It needs the search text to appear exactly once, refuses migrations, generated code and tests, restores the file byte-for-byte on every exit path (and proves it with `cmp`), and runs only the tests you name.

**Database rule** (trigger, grant, RLS, constraint): `scripts/mutate-db.sh 'MUTATE SQL' 'RESTORE SQL' TEST_REGEX`
Example: `scripts/mutate-db.sh 'GRANT DELETE ON students TO app_rt' 'REVOKE DELETE ON students FROM app_rt' 'TestINV_TheAPIRole'`
It only runs against a local database, always runs the restore SQL, and re-runs the tests after restoring to prove the database is back. Use only statements that change rules, never ones that insert rows: append-only tables cannot be cleaned afterwards.

**Results.** Exit 0 = KILLED (a test failed: the rule is protected). Exit 1 = SURVIVED (nothing failed): write the missing test, run the same mutation again, expect KILLED. Exit 2 = the harness refused or failed: read its message.

**What to mutate** (choose the line that carries the rule, not a comment): a comparison that enforces a limit; a `Require` call in a handler; a `WHERE tenant_id` or a policy; a unique index that stops a duplicate; a trigger that forbids a change; an overflow check; the filter that picks the latest version.

**Never** mutate on a shared or remote database, never edit a test to make a mutation die, and never leave a mutation in place: after any run, `git status` must show only your intended changes.

Report each mutation as: rule, mutation, KILLED or SURVIVED, and the test that caught it (or the test you added).
