---
name: plan-slice
description: Write a short plan for one slice and stop for the owner's approval before any code. Use for any feature, module or change larger than a small fix, and always for money, security or schema work.
argument-hint: "[slice, e.g. challans: generate, review, release]"
disable-model-invocation: true
allowed-tools: Read Glob Grep Bash(git *) Bash(ls *) Bash(make spec-coverage *) AskUserQuestion
---
Plan the slice: $ARGUMENTS

1. Run `/dev-context` for the area if you have not in this session.
2. Write the plan. Keep it to one screen and use exactly these parts:
   - **Goal** in one sentence, and what moves (money? data? only setup?).
   - **Requirements covered**: REQ/BR/US IDs. **Explicitly not covered**: IDs left out and why.
   - **Design in five lines**: tables, rules, endpoints, permissions, which modules it asks (via `pgx.Tx` methods, ADR-0013).
   - **The riskiest part** and how it will be proven (which invariant, which mutation check).
   - **Order of work** (the build-slice steps) and the commits you expect.
   - **Decisions I need** (see step 3).
3. Collect decisions. For each one, state your recommendation and the trade-off. Ask with `AskUserQuestion` (2 to 4 options, recommended first) when the answer changes what you build. Always ask for: anything involving money rules, a new public endpoint, a permission model change, a new dependency, a breaking API change, a schema change that is hard to reverse, and any conflict between the spec, the code and what the owner said (see `/ask-human`). Do not ask about naming, file layout or anything the standards already settle: state the assumption instead.
4. **Stop.** Do not edit code, migrations or the contract until the owner approves. Say what you will do first on approval.
5. After approval, record decisions: a DEC row in `docs/spec/01-decisions/01-decision-log.md` if the spec changes, an ADR (`/adr`) for a technical choice.

A good plan is short, names its risks, and makes the owner's yes or no easy.
