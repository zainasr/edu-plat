# ADR-0021: Advance, refunds and defaulters

| | |
|---|---|
| Status | Accepted (choices made by the product owner on 2026-11-17) |
| Date | 2026-11-17 |
| Spec links | REQ-FEE-023, REQ-FEE-041, REQ-FEE-042, DEC-010, DEC-028, DEC-029, BR-FEE-008, ADR-0014, ADR-0020 |

## Context
Parents pay more than is due, pay ahead, and sometimes want money back. DEC-010 says there is no free-floating family credit: an overpayment is held against the next challan, and what cannot be applied is refunded explicitly. The office also needs to know who is overdue and whom to call.

## Decision
1. **Advance belongs to a payment.** `payments.advance` is the part of a payment held for the family's next challans. It posts credit Advance Fees Received (a liability). What is left is worked out: advance minus applications minus refunds; nothing is stored.
2. **It is held only when asked** (`hold_excess_as_advance`). Without the flag a payment above the dues is refused as in 5a, so a typing slip cannot park money. With nothing owed and the flag set, the whole payment is advance.
3. **Applied automatically at release**, oldest payment first, never beyond what the challan owes or the payment holds: debit Advance, credit Fee Receivable, in the release transaction, with one `advance_applications` row per payment used and one audit entry.
4. **Refunds are explicit and one-step.** `POST /payments/{id}/refund` (permission `fee.refund`: Accountant, Owner) posts debit Advance, credit Cash or Bank, writes an `advance_refunds` row and an audit entry, and cannot exceed what is held. Maker-checker approval is a later slice (DEC-029).
5. **A payment cannot be voided once any advance has been applied or refunded** (`payments.advance_in_use`): that money has moved on. The database enforces it too (the payments guard trigger), even for the table owner.
6. **Three rules are enforced by the database at commit**, extending ADR-0020: allocations plus advance equal the payment amount; a challan is never paid beyond its total counting allocations and applied advance, and only by its own family; an advance is never spent (applied or refunded) beyond what the payment held. Applications and refunds are append-only for every role.
7. **Lock order.** Release locks its draft challans, then the family's advance payments, and only then takes counters. Refund and void lock the payment row, then the journal counter. So no two of them can wait on each other. Tests race a release against a refund and against a void of the same payment, many rounds.
8. **Defaulters are derived, never stored**: families with a released challan that fell due at least `min_days` ago (default 30, 1 to 365) and still has a balance, most overdue first, with the overdue challans and the fee payer's name and mobile (the guardian marked fee payer, else the primary one, from the students module). A payment removes a family from the list at once.

## Notes from review
- **A payment is final once advance has moved.** Once even a paisa of its advance is applied or refunded, the whole payment (including its challan allocations) can no longer be voided. A mistaken receipt in that state has no correction path in this slice; refund the unused advance and, if the rest is wrong, a later correction (adjustment) slice must handle it.
- **A family with unspent advance and an overdue challan still appears as a defaulter**, because advance is applied only at release, never to challans already open (DEC-010). Cashiers may find this surprising; the dues view shows the advance next to what is owed.
- **Defaulters paging** is keyset on (oldest due date, family). If the oldest challan is paid between two page fetches the family's key moves later and it can appear again on a later page; the cutoff also moves at midnight. Acceptable for a worklist.
- **PII:** the call list shows the fee payer's mobile to anyone with `fee.view` (Owner, Accountant, Receptionist). Open question for the owner whether that should need a narrower permission.
- The refund account need not match the payment's own account (cash received can be refunded from the bank).

## Not in this slice
Security deposit adjustment and refund, late fees, reminders and WhatsApp, CSV export, contact notes and cash sessions (slice 5c); maker-checker approval.

## Consequences
- The advance a family holds is visible on its dues and on each payment (applied, refunded, available).
- Release does more work when a family holds advance, in the same transaction; a failure rolls everything back.
- The defaulters query is a join over challans and allocations; fine for a school's volume, and indexed by tenant.
- The call list shows a mobile number to anyone with `fee.view`; today that is Owner, Accountant and Receptionist.
