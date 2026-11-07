# ADR-0015: Fee setup as effective-dated, append-only versions

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-07 |
| Spec links | REQ-FEE-001, REQ-FEE-002, BR-FEE-001, BR-FEE-004, DEC-011, ADR-0004, ADR-0013, ADR-0014 |

## Context
A class fee plan decides what every challan charges. The spec says plan changes never alter released challans, apply from an effective month, and are audited (REQ-FEE-001, BR-FEE-001). A student can also carry an override per fee type. Challan generation (the next slice) must be able to ask "what was this student charged in month M?" and get the same answer today and next year.

## Options considered
1. **Edit the plan line in place, with an audit entry.** Simple to read. The old amount survives only in the audit log, and any query that re-derives an old month can silently use the new amount.
2. **Effective-dated, append-only versions.** A change adds a row that takes effect from a month. The amount for a month is the latest version on or before it. Nothing is overwritten.
3. **Copy the whole plan per session.** Easy to reason about, but it duplicates every line each year and does not express "from October".

## Decision
Option 2, for both class plan lines (`fee_plan_lines`) and student overrides (`student_fee_overrides`).
- A row is one version: fee type, amount in paisa, frequency, and `effective_from` (always the 1st of a month, enforced by a `CHECK`).
- `UNIQUE (tenant, class or student, fee type, effective_from)`: one version per month. A second is a 409; the user picks a later month.
- A **removal** is a version with `removed = true` (amount 0). A **cleared** override is a version with `cleared = true` (the student returns to the plan amount). A zero amount on an override is a real full concession, so "cleared" is a separate flag, not amount 0.
- Both tables are append-only for every role: no `UPDATE` or `DELETE` grant to the API role and a `forbid_mutation` trigger that also stops the owner. Fee types may change only `status` (a column-level `UPDATE` grant) and are never deleted.
- Resolution is a pure function (`resolvePlan`, `resolveStructure`) over the stored versions. It skips removed and archived types, orders by name, and lets an override replace the plan amount only for a fee type that is on the plan. An override for a type not on the plan is ignored, not turned into a charge.
- The resolver returns each line with its **frequency** and does not decide whether a line falls due in a month. Monthly, one-time at admission, annual and ad hoc are challan generation's concern (REQ-FEE-003), where the admission date and the school's annual month are known.
- Suggested fee types are created the first time a school lists its types (`INSERT ... ON CONFLICT DO NOTHING`, like the default branch in ADR-0013). An archived suggestion is not recreated.
- The fees module asks `org` whether a class exists and `students` which class a student is in, through methods that take the caller's transaction (ADR-0013). It reads neither module's tables.

## Consequences
- Good: history is exact; a later change cannot alter an earlier month (proved by a unit test and, with the filter deliberately broken, by three failing tests); released challans can never be altered by a plan edit; the same append-only rule as the ledger.
- Bad: a correction to the same month is refused (409) rather than edited; the user adds a version for a later month, or the school asks for a reversal workflow later. Reading a plan loads every version of a class (a handful of rows per year per fee type), which is fine until classes carry hundreds of versions.
- Follow-ups: challan generation must decide which frequencies fall due and must copy the resolved amounts onto the challan lines, so the challan never re-reads the plan. Discounts (sibling, scholarship) and late fees are separate slices. A "correct this month" flow with approval may be needed once schools use it.
