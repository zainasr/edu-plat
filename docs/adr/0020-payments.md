# ADR-0020: Payments at the counter

| | |
|---|---|
| Status | Accepted (choices made by the product owner on 2026-11-17) |
| Date | 2026-11-17 |
| Spec links | REQ-FEE-020 to 025, BR-FEE-002, BR-FEE-003, BR-FEE-004, BR-FEE-008, US-FEE-002 to 004, REQ-API-010, ADR-0004, ADR-0014, ADR-0017, ADR-0018 |

## Context
A receptionist takes cash for a family; the money must reach the right challans, be numbered without gaps, reach the books, and be undoable without erasing anything. Challans are immutable once released (BR-FEE-001), so "paid" cannot be written onto them.

## Decision
1. **What is paid is worked out, never stored on the challan.** A challan's paid amount is the sum of the allocations of payments that stand (not voided). Voiding a payment therefore reopens its challans with no update to them.
2. **One payment is one receipt.** `payments` holds family, amount, method (cash, bank transfer, cheque), the account it is deposited to (1000 Cash in Hand or 1100 Bank), date and the journal entry; `payment_allocations` split it across challans. A family payment for three children is one numbered receipt with a line per child (DEC-026).
3. **Allocation** is oldest due date first (ties by challan number), or exactly as the cashier chooses; either way the parts add up and no line exceeds what its challan owes. A payment above what the family owes is refused (`payments.exceeds_dues`) until Advance exists (DEC-027).
4. **Two rules are enforced by the database at commit**, like the ledger's balance (ADR-0014): a payment's allocations add up to its amount, and a challan is never paid more than its total by payments that stand. Payments change once (recorded to voided, void columns only); allocations never change; neither can be deleted.
5. **Receipt numbers** are `RCP-<fiscal year>-NNNNNN`, gapless per school and fiscal year (July to June, named by the starting year), from `tenant_counters` inside the payment transaction, so a refused or rolled-back payment gives its number back. A voided receipt keeps its number, and the number is never reused.
6. **Posting.** One entry per payment: debit Cash or Bank, credit Fee Receivable, in the same transaction. Void posts the exact reversal through `ledger.Reverse` and marks the payment voided.
7. **Concurrency.** A payment locks every released challan of the family, in id order, before reading balances, so two cashiers cannot both see the same open balance. The database trigger is the second line. Void locks the payment row, so it runs once.
8. **Idempotency-Key is required** to take a payment (ADR-0018). The handler checks it after the permission, so callers without `fee.collect` still get 403.
9. **Security deposits are a liability from release** (DEC-025). Slice 3c wrongly credited every challan line to Fee Income; release now credits deposit lines to Security Deposits Held and the rest to Fee Income, and a test proves it. This is fixed forward: challans released before this change (none outside development) are not restated.
10. **Permissions:** `fee.collect` for Receptionist, Accountant and Owner; `payment.void` for Accountant and Owner. Maker-checker approval of voids (BR-IAM-003) is optional per school and not built.

## Found in review
- **A lock gap.** The first version locked the family's released challans in one statement and read balances in another, so a challan released between the two could be paid without its lock. Now balances are read for exactly the challans that were locked; a challan released a moment later is simply not part of that payment. A stress test races payments against a release (12 rounds) and fails if any challan is over-paid; a mutation check that reads all challans again is killed.
- **The database now also checks that the payment's family is the challan's family**, so a bug or hand-written statement cannot allocate across families. The commit-time triggers are a backstop behind the application locks, not a substitute for them.
- **A commit-time refusal is a 409 `payments.conflict`** ("nothing was recorded; try again"), never a 500, and so is a deadlock or serialization failure.
- **Receipt numbers follow the payment's date, not the day it is entered.** A payment dated 30 June but recorded on 2 July takes the next number of the old fiscal year, so numbers are gapless within each year but not chronological across the year boundary. This follows REQ-FEE-024 literally; an owner who prefers numbers by entry date can change it before go-live.
- Challans released before this change (development data only) booked deposits as income and are not restated.

## Not in this slice
Advance and refund (next), cash sessions (REQ-FEE-026), online payments, late fees, print layouts, reminders and defaulters, and a search endpoint for the counter screen (find a family by name or mobile).

## Consequences
- Dues and balances are read with a join over allocations; an index on `(tenant_id, challan_id)` serves it. Fine for a school's volume.
- Because the balance is derived, a payment dated before its challan's release is possible; no period locks exist yet.
- The receipt number sequence takes one counter row, so payments in the same school serialise briefly at that row (as journal entries already do).
