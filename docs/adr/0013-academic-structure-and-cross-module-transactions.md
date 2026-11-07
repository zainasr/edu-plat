# ADR-0013: Academic structure, the default branch, and cross-module calls inside one transaction

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-07 |
| Spec links | REQ-ORG-001, 005, 006, 008, BR-ORG-001, 003, REQ-STU-001, 008, US-STU-005, DEC-011, ADR-0001, ADR-0002 |

## Context
Students must be enrolled in a section of the current session (REQ-STU-001), and fees will attach to classes. The spec's hierarchy is Organisation > Institution > Branch > (Session, Class > Section), but the 1.0 UI shows one institution. Enrolling a student needs facts that belong to the org module (is the section active, which session is current), and the enrolment must commit together with the student. The repo rule is that a module never reads another module's tables.

## Options considered
**Hierarchy**
1. **Branches only now; add the institution level later** by a backward-compatible migration. Smallest change, no unused tables.
2. **The full hierarchy now.** Matches the spec's diagram, but adds tables and endpoints nothing uses yet.

**Where the default branch comes from**
1. **Create it the first time the org module writes** (`INSERT ... ON CONFLICT DO NOTHING` on a partial unique index). Existing tenants need no backfill; `cmd/bootstrap` is unchanged.
2. **Create it in `Bootstrap`** and backfill existing tenants in the migration. `iam` would then write org's table, or import org and create a dependency cycle.

**How `students` asks `org` a question**
1. **Pass the caller's open `pgx.Tx` into an exported org method** (`ResolveEnrolmentTarget`, `DescribeSections`, `DescribeSessions`). One transaction, one commit; org owns its SQL.
2. **Call org through its own transaction.** Two transactions: a section could be archived between the check and the insert, and a failed enrolment would leave the first transaction's effects.
3. **Join org's tables from the students queries.** Fastest to write; breaks module ownership.

## Decision
- Branches only now (option 1). The institution level is added later with an `institution_id` column and a backfill.
- The default branch is created lazily on org writes (option 1). Reads (`List*`) never create it, and a tenant with no branch lists nothing. A concurrency test proves six simultaneous writers produce exactly one branch.
- Cross-module calls take the caller's tenant transaction (option 1). The signature `func (s *Service) ResolveEnrolmentTarget(ctx, tx pgx.Tx, tenantID, sectionID string)` documents it: the callee must not begin or commit.
- The `enrolments` table belongs to `students` (history of a student); it stores section and session ids, and the names come from org at read time.
- No DELETE grant on any structure table or on `enrolments` (DEC-011). Classes and sections are archived. Foreign keys without `ON DELETE` actions make "a section with enrolments cannot be deleted" (BR-ORG-003) true even for the owner role.

## Consequences
- Good: one transaction covers the check and the write; org's SQL stays private; no backfill; the schema forbids the destructive actions the business forbids.
- Bad: the exported org methods taking `pgx.Tx` leak the database type into a module's API. They are the only ones, and are marked in the code. A reader must remember that callee code runs inside the caller's transaction.
- Follow-up: when the institution level arrives, add it with a migration that expands (new nullable column, backfill) before it contracts. Revisit the `pgx.Tx` parameter if a third module needs the same pattern.
