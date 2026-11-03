# Module 04. Fees and Payments

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 (term/quarterly cycles, bank statement auto-match: 1.1) |
| Module code | FEE |

## 1. Purpose
The money heart of the product: define fee structures, generate challans safely (draft → review → release), collect at the counter or online, track dues and defaulters, and post everything to the accounting engine.

## 2. Concepts
- **Fee head (type):** tuition, admission, registration, annual/exam, transport, lab, library, sports, fine, security deposit, custom. Defaults are suggestions, never locks; schools add their own and switch types off. (ref)
- **Fee plan:** per class, a list of heads with amount and **frequency** (monthly, one-time at admission, annual, ad hoc). Frequency is on the head, not on the whole plan. Fee structure differs between institutions: we ship a default and everything is customisable per institution.
- **Student override:** per head and discount.
- **Challan:** a voucher with lines, amount, due date and a unique consumer number.
- **Family billing:** challans roll up to a family; one payment can cover several children.

## 3. Lifecycle

```
Challan:  draft ──review──> released ──> partial ──> paid
              │                 │
              └─ void           └─ void (reversing entry)
(overdue and defaulter are derived from due date, not stored states)

Payment:  recorded ──> voided (reversing entry; number kept)
Online:   initiated ──> succeeded | failed ──> (webhook reconciliation)
```

## 4. Requirements

### 4.1 Plans and generation
- **REQ-FEE-001** Fee plan per class with head, frequency; student-level override per head; effective-from month. Plan changes never alter released challans; they apply to drafts and future amount and frequency; student-level override per head; effective-from month. Plan changes never alter released challans; they apply to drafts and future months, with an adjustment line if needed. Plan changes are audited.
- **REQ-FEE-002** Monthly + one-time + annual frequencies in 1.0; quarterly/term in 1.1.
- **REQ-FEE-003** **Auto-draft** of next month's challans on a configurable day (default the 25th); the due date defaults to the 10th (configurable). Manual **Generate Fee** is also available and **idempotent**: students who already have a challan are skipped; the preview shows "Has challan" vs "Will generate". (ref)
- **REQ-FEE-004** **Review and release:** an Admin/Accountant reviews the drafts with an exceptions list (new admissions, discount changes, mid-month joiners) and releases in bulk. Parents see only released challans.
- **REQ-FEE-005** Opening dues: "Remaining Dues" entered per student at import or manually as a dated receivable that appears on challans and in aging. (ref)
- **REQ-FEE-008** Challans, receipts and reminders are addressed to the student's **fee payer** guardian (default the primary), who may be an uncle, grandparent or other relative; the receipt shows the payer's name and relationship.
- **REQ-FEE-006** Challan numbering and a unique **consumer number** (1Bill/1Link style) per challan so it can be paid from any bank app.
- **REQ-FEE-007** Challan layouts for print: A4 (multi-copy) and A5; includes bank accounts and payment instructions.

### 4.2 Discounts and late fees
- **REQ-FEE-010** Discount types: sibling, staff-child, scholarship (fixed or %), concession (need-based), custom. Each has a scope (tuition only or all heads), a duration (until date/session), an approval flag and a report.
- **REQ-FEE-011** Stacking: apply the highest single discount unless the institution allows stacking (default: no stacking).
- **REQ-FEE-012** Sibling discount configurable per institution (for example 2nd child 10%, 3rd child 20%), tuition only, order by admission date.
- **REQ-FEE-013**  late fees is common in pakistan with no charges and it should be customizable if need charges per school.

### 4.3 Collection
- **REQ-FEE-020** Counter collection screen: search by name / registration / mobile / family; tabs All outstanding, Overdue, Partial, Defaulters, Family billing; select challans or whole family; partial payment allowed. (ref)
- **REQ-FEE-021** Payment methods: cash, bank deposit/transfer (manual with reference), cheque, plus online via the aggregator (JazzCash, Easypaisa, bank/Raast QR). Every payment names the **account** it is deposited to (for example "Cash in Hand"). A **payment method** field is mandatory (the reference leaves it "not stated"). (ours)
- **REQ-FEE-022** One family payment is allocated across children; child-level receipts plus a combined receipt are produced. The default allocation is oldest challan first, editable by the user.
- **REQ-FEE-023** Advance: an overpayment is applied to the next challan(s) and shown as Advance; money that cannot be applied must be refunded explicitly. There is no free-floating credit balance. (DEC-010)
- **REQ-FEE-024** Receipts are numbered `PREFIX-YEAR-NNNNNN` per institution per fiscal year, sequential, no gaps; formats A4, A5, 80 mm thermal (browser print); reprints are marked "Duplicate"; the family receipt lists sibling lines. (ref)
- **REQ-FEE-025** Voiding a payment keeps the receipt number, requires a reason and approval (if maker-checker on) and produces a reversing journal entry. Voided receipts stay visible via a "Show voided" filter. (ref)
- **REQ-FEE-026** Cash sessions: each collector opens a session, records collections, closes with a count and variance note; "day close" report (see Accounting).

### 4.4 Online payments
- **REQ-FEE-030** Optional per school; OFF by default until the school completes aggregator onboarding. Funds settle to the school's bank account.
- **REQ-FEE-031** Payment reconciliation by webhook (signature verified, replay safe, idempotent). Unmatched payments go to an "unallocated" queue for staff to assign.
- **REQ-FEE-032** Bank slips: manual entry now; statement CSV upload with auto-match is [1.1].
- **REQ-FEE-033** Parents can pay all or selected challans and see status in real time.

### 4.5 Follow-up
- **REQ-FEE-040** Reminder schedule configurable: 3 days before, due day, +3, +7, +15 via push and WhatsApp (important events); escalation to an owner list at +30.
- **REQ-FEE-041** Defaulter = 30 days or more overdue (configurable). Optional school policies (off by default): hide report card, flag at gate, auto-status "Defaulter". No automatic strike-off. (ref)
- **REQ-FEE-042** Defaulter worklist: call list with last contact note, bulk select overdue/defaulters, send reminder, export.

### 4.6 Security deposits
- **REQ-FEE-050** Security deposit is a refundable fee type "held on the family's behalf". It is a **liability**, never income. A Security Deposits page lists held, adjusted and refunded amounts. Refund requires approval and posts ledger entries.
- **REQ-FEE-051** (ours) The schedule view and the ledger MUST agree. Only **received** deposits are held as a liability; billed-but-unpaid deposits are a receivable. (The reference shows a mismatch between its schedule and ledger.)

### 4.7 Other
- **REQ-FEE-060** Ad hoc fees (books, uniform, events, fines, exams) as extra fee heads through an Additional Fees page. No inventory or shop in 1.0.
- **REQ-FEE-061** Invoices (custom or automated) for single students or families.
- **REQ-FEE-062** Tax lines: not generated in 1.0; schools remain responsible; revisit with an accountant.
- **REQ-FEE-063** Fee history per student/family: every challan, payment, adjustment, with who and when.

## 5. User stories

### US-FEE-001 Monthly cycle
As an **Accountant** I want drafts prepared automatically so that I only review and release.

- Given auto-draft is on and today is the 25th, when the job runs, then draft challans exist for all active students for next month with due date the 10th.
- Given a student already has a challan for the month, when I run Generate Fee manually, then that student is skipped and listed as "Has challan".
- Given I release the drafts, then guardians receive a "challan released" notification and see the challan in the app.

### US-FEE-002 Collect at the counter
As a **Receptionist** I want to take a payment in under 30 seconds.

- Given I search by guardian mobile, when I choose the family and enter Rs 12,000 cash into "Cash in Hand", then the system allocates it oldest-first, prints a receipt and shows updated dues.
- Given I pay more than the family owes, then the extra is held as Advance against the next challan and shown on the receipt.

### US-FEE-003 Family payment
- Given a family with 3 children and 3 unpaid challans, when I take one payment of 25,000, then each child's challan is reduced per allocation, three child receipts and one combined receipt are produced.

### US-FEE-004 Void a payment
- Given a payment of 5,000 recorded by mistake, when I void it with a reason, then (if maker-checker is on) it waits for another approver; after approval the challan reopens, a reversing entry is posted and the receipt number remains with a "voided" mark.


### US-FEE-005 Parent pays online
As a **Parent** I want to pay a challan from my phone.

- Given online payments are enabled, when I choose JazzCash and complete payment, then within a minute the challan shows paid, a receipt is available and the school's books show the payment.
- Given a payment succeeds at the aggregator but the webhook is delayed, then the challan shows "payment processing" and reconciles on webhook or manual match.

### US-FEE-006 Defaulter follow-up
- Given students 30+ days overdue, when I open Defaulters and select all, then I can send a WhatsApp reminder and export the call list.

### US-FEE-008 Refund security deposit
- Given a student withdraws with a Rs 5,000 deposit and Rs 1,000 dues, when I process the deposit, then I can adjust Rs 1,000 and refund Rs 4,000 with approval, creating ledger entries.

## 6. Business rules
- **BR-FEE-001** Released challans are never edited; corrections use adjustment lines or void-and-reissue.
- **BR-FEE-002** Receipt numbers are sequential without gaps; voiding keeps the number.
- **BR-FEE-003** Payments are never deleted; they are voided with a reversing entry.
- **BR-FEE-004** Security deposits are liabilities, not income.
- **BR-FEE-005** Holidays suppress due reminders.
- **BR-FEE-006** Withdrawn students get no new monthly challans; existing dues remain collectable.
- **BR-FEE-007** A discount applies by the stacking rule; the highest single discount wins by default.
- **BR-FEE-008** Every financial action posts a balanced journal entry; failures surface on "Needs attention".

## 7. API (representative)
`/fee-types`, `/fee-plans`, `POST /challans/generate`, `POST /challans/release`, `GET /challans?filter[status]=`, `POST /payments`, `POST /payments/{id}/void`, `/receipts/{id}`, `/security-deposits`, `/defaulters`, `POST /payments/online/initiate`, `POST /webhooks/payments/{provider}`.

## 8. Events
`EVT-FEE-challan.released`, `payment.recorded`, `payment.voided`, `payment.online_succeeded`, `latefee.applied`, `deposit.refunded`.

## 9. Decide in design
Aggregator selection; consumer-number scheme (1Bill/1Link); challan template design; reconciliation of unallocated queue UX.
