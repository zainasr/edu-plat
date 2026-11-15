# ADR-0017: Challans: drafts post nothing, release posts the receivable

| | |
|---|---|
| Status | Accepted (decisions chosen by the product owner on 2026-11-15) |
| Date | 2026-11-15 |
| Spec links | REQ-FEE-003, REQ-FEE-004, REQ-FEE-006, BR-FEE-001, BR-FEE-006, BR-FEE-008, ADR-0004, ADR-0014, ADR-0015 |

## Context
A challan is the monthly voucher a school sends for each student. It must be drafted safely, reviewed, then released in bulk. Once released it is a financial fact and must never change (BR-FEE-001), and the school's books must show what it is owed (BR-FEE-008). The spec leaves open when the receivable is booked, how a bank-app consumer number is formed, and which plan lines fall due in a month.

## Decision
1. **A draft posts nothing. Release posts the receivable**: debit 1200 Fee Receivable, credit 4000 Fee Income, through `ledger.Post` in the same transaction that marks the challan released. A draft that is voided never touched the ledger, so it needs no reversal. Voiding a released challan is a ledger reversal and comes with payments.
2. **Immutability is enforced by the database**, not by code alone: a trigger refuses any update to a challan that is not a draft and any change to its identity, month, due date or total; the lines are append-only; the API role has no DELETE and may update only the release and void columns.
3. **Lines are copied from the fee structure when the draft is made** (name, kind, frequency, amount), so a later plan change cannot alter a draft (BR-FEE-001, ADR-0015).
4. **One live challan per student and month**: a unique partial index (`status <> 'void'`). Generating twice is safe, including concurrently.
5. **Which lines fall due**: monthly lines every month; one-time lines in the month of the student's first enrolment; annual lines in the month of the year the plan line first took effect; ad hoc lines never (they belong to the Additional Fees page, REQ-FEE-060). A zero line (a full concession) is left off; a student with nothing left to bill gets no challan.
6. **Who is billed**: students with an active enrolment that began on or before the month's last day. A student with no active enrolment is not billed, and their existing dues stay collectable (BR-FEE-006).
7. **Numbers**: the challan number is gapless per school, taken from `tenant_counters` at release (a failed release gives it back). The consumer number is 14 digits: the challan number padded to 13 digits plus a Luhn check digit. It is unique within a school, not yet across schools, because the aggregator's real scheme (1Bill/1Link) is still an open question. When it is chosen, only `consumerNumber` and a prefix column change; no challan is rewritten.
8. **Release is all or nothing** for a batch of up to 200 challans, locking rows in id order so overlapping releases cannot deadlock.

## Options considered
- Post on generation: books the draft too, so every void needs a reversal and the ledger fills with noise. Rejected.
- Tenant code in the consumer number: the `tenants` table is not readable by the API role. A global prefix needs a new definer function (ADR-0012) and an aggregator decision first. Deferred.
- Partial release (skip bad challans): hides errors from the accountant. Rejected for now.

## Consequences
- The ledger only ever holds receivables for challans a person approved.
- Correcting a released challan is a later slice (void-and-reissue or an adjustment line with a reversal).
- Mid-month joiners are billed in full for the month and appear as `new_admission` on the review list; proration is not built.
- The auto-draft job on the 25th is not built; it needs the job queue (ADR-0005, still Proposed). Generate is manual and idempotent, so the job can later call it.
- Students currently have only the status `active`; "withdrawn" is expressed by a closed enrolment until the lifecycle slice adds statuses.

## Lock order (found in review)
Release locks every challan in the batch, in id order, before it takes any number. The number counter is one row per school and is held until commit, so taking it between challan locks let two overlapping batches wait on each other. `TestREQ_FEE_004_OverlappingReleaseBatchesDoNotDeadlock` runs 30 rounds of overlapping batches and fails if the old order returns (mutation checked).

## Owner decisions after review (2026-11-15)
1. **Idempotency-Key on release (REQ-API-010): in the next slice**, built once for release and payments. Until then a retried release gets 409 `challans.not_draft`; no double posting is possible (row lock plus the draft condition).
2. **Void of a draft uses `fee.generate`.** The catalogue has no void-challan permission; a draft changes no money.
3. **The exceptions list is recomputed, not stored.** `new_admission` is set on every challan read from the student's first enrolment date (students module, ADR-0013). Discount-change and mid-month-joiner exceptions are not built.
4. **Month bounds:** past months are allowed (backfill); a month more than one ahead of today (Pakistan time) is refused with `fees.generate_invalid` on `month`. Books are still dated the release day.
5. **Built-in roles Accountant and Receptionist** were added (migration 00009, `iam/principal.go`) and `TestREQ_IAM_005_BuiltInRolesHoldTheirPermissions` checks every role against every endpoint it should and should not reach. This closed the gap where `fee.release` could be swapped for `fee.view` unnoticed. Each new endpoint adds a row to that test.

## Known limits
- Generate runs the whole school in one transaction with per-student queries. Fine for hundreds of students; revisit with the job queue (ADR-0005).
- Challan books date is the release day, not the challan month.
