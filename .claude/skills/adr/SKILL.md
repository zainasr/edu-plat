---
name: adr
description: Create a new Architecture Decision Record. Use when a significant technical choice is made or changed (library, pattern, infrastructure, data model).
argument-hint: "[short title]"
disable-model-invocation: true
allowed-tools: Read Write Bash(ls *)
---
Create an ADR for: $ARGUMENTS

1. List `docs/adr/` and take the next number (`NNNN`).
2. Copy the structure of `docs/adr/0000-template.md` into `docs/adr/NNNN-kebab-title.md`.
3. Fill Context (forces, constraints from the spec), Options considered (at least two, with honest trade-offs), Decision, Consequences (good, bad, follow-ups). Link REQ/DEC IDs and any superseded ADR.
4. Status is `Proposed` until the product owner accepts it; then `Accepted`.
5. Add a line to `docs/adr/README.md`.
Keep it to one page. Write plain language.
