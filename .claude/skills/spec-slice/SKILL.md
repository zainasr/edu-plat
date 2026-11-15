---
name: spec-slice
description: Superseded by the agentic workflow. Use /plan-slice, then /build-slice, then /verify, then /ship. Kept so the old name still points somewhere.
argument-hint: "[module or story ID, e.g. US-FEE-002]"
disable-model-invocation: true
---
`/spec-slice` is replaced by one procedure in four steps, each with its own skill:

1. `/plan-slice $ARGUMENTS`: a short plan; stop for the owner's approval.
2. `/build-slice $ARGUMENTS`: contract, migration, queries, rules, service, handlers, tests.
3. `/verify $ARGUMENTS`: independent evaluation with agents and mutation checks.
4. `/ship $ARGUMENTS`: small commits, approved push, CI.

Start with `/dev-context` if this is a fresh session. The full explanation is in `docs/engineering/development-workflow.md`.
