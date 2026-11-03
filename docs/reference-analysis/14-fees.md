# 14 Fees (Fee hub, Collect, Challans, Payments, Invoices, Fee types, Settings)
Routes under /admin/fees: (hub), /collect, /challans, /payments, /advance, /additional, /history, /invoices, /print-challans, /activity, /generate, /deposits, /types, /settings. Screenshots: screenshots/14-fees/. Student/parent names in demo data are not copied here.

## Hub (/admin/fees) — monthly cockpit
- Header: month (October 2026), banner "Auto-generation on: draft challans for next month are created automatically on the 1st at 6:00 am for you to review" (links to settings), button **Generate October Fee**.
- Tiles: Collected (PKR 88.7K, 18% of PKR 499,930 billed this month), Outstanding (PKR 411.2K, 165 students, all time), Overdue (198 challans, PKR 286K), Defaulters (6 = 30+ days overdue).
- Month panel: collection-rate bar; **challan status counts**: Generated (202 of 208 expected), Draft (pending approval), Paid (+partial), Unpaid (released). All-time summary: collected vs outstanding, split Overdue vs Not-yet-due.
- Quick actions (13): Review Challans, Collect Fee (students with dues), Payments Received, Advance Fee, Additional Fees, Fee History (defaulters), Download Invoice, Print Challans, Needs Attention, Generate Fee, Security Deposits, Fee Types, Fee Settings.

## Challan lifecycle
Auto/ manual generation -> **Draft** -> admin review & approve ("Review Challans") -> released **Unpaid** (due date, default due on the 10th) -> **Partial** -> **Paid**; overdue computed from due date; Defaulter = 30+ days overdue. Challans include different fee lines (Monthly Tuition, Remaining Dues from import, Security Deposit, Other). Review page empty when no drafts.

## Collect Fee (/collect)
Counter-collection screen: bulk selectors (Select All Outstanding, Overdue, Defaulters), stats, filters (month, fee type, class, section), segmented tabs (All Outstanding, Overdue, Partial, Defaulters 30d+, **Family billing**), list "Outstanding by student" with reg no, class/section, number of fees, days overdue, father name + phone, amount, view statement icon and **Collect** button, Export CSV/PDF. Family billing groups siblings to pay together.

## Payments Received (/payments)
Ledger of all payments: tiles (collected, at counter, online via gateway); filters (today/7 days/month/last month/all, date range, counter vs online, method, recorded-by, class, section, show voided); breakdown "paid through" and "recorded by"; table: paid on, student (+N siblings, fee lines count), method (Counter, deposited into account e.g. "Cash in Hand"), recorded by, reference/receipt no (APH-YYYY-NNNNNN), amount; per-row download, print, **void**.

## Advance Fee (/advance)
4 steps: student, month (current to 12 months ahead), fee amount, collect (amount, date, method Cash/Bank Transfer/Online/Cheque/Card, reference, notes). Creates the future challan if missing then records payment.

## Additional Fees (/additional)
One-off charges "applied instantly as Unpaid challans". Types: Fine, Examination, Event, Transport, Library, Sports, Lab, Admission, Registration, Security deposit, Remaining dues, Other. Fields: title*, amount*, due date, notes; "Apply to" individual, multiple, class, and (likely) whole school.

## Invoices (/invoices) and Print Challans
- Invoices: every receipt/invoice issued, filters class/section/period/fee type/invoice type (Single or bulk payment, Family), Automated vs Custom tabs, "Create custom invoice", download as PDF stack in A4, US Letter, A5, or 80 mm thermal receipt. Numbering APH-2026-000051.
- Print Challans: one challan per student listing everything owed, downloaded as one PDF per class/section; setting decides whether challans are whole-account or monthly.

## Fee Types (/types)
14 types: Monthly Tuition (amount from class), Admission Fee 500, Registration Fee 500, Security Deposit (refundable, held for family, returned when student leaves), Transport, Library, Sports, Laboratory, Examination, Event, Fine, Remaining Dues, Other, plus school-defined. Each: default amount (suggestion only), behaviour (Monthly / On admission / Charged as needed / Refundable), on/off, usage count.

## Fee Settings (/settings) five cards
Automatic Generation (schedule, drafted on 1st due 10th), Fee Challan layout (template "Classic"), Bank Accounts (which account challans name; default "pay at the school office"), Invoice Settings (receipt number format), Online Payments (parents pay in app or via any bank; **not set up** on trial).

## Other
Needs Attention (/activity): automated health checks (failed payment numbers, bank-taken-but-unrecorded money, accounting entries that failed to post). Pages /generate, /deposits, /history did not finish loading in the session (spinner) so contents are inferred from titles: manual fee generation by class/student, security deposit holding/refund, defaulter history.

## Entities
FeeType, ClassFeePlan (standard monthly fee on class), StudentFee (discounted fee, discount type), Challan(student/family, period, status, due_date), ChallanLine(fee_type, amount), Payment(challan lines, method, account, recorded_by, receipt_no, voided), Invoice(receipt) with family grouping, Deposit, BankAccount, FeeSettings.

## Observations / gaps / ideas
- Strong: draft-then-approve automation, family billing, remaining-dues migration, per-line payments, void with audit, accounting integration (payments post to accounts like "Cash in Hand").
- Gaps vs Pakistan: all payments entered "Not stated" method (staff leave method blank); no JazzCash/Easypaisa/1Link/bank-challan reconciliation; online payments not configured; no SMS/WhatsApp reminder visible here; no instalments, late-fee rules, scholarship/concession approval flow seen here (concessions report exists).
- Ideas: auto reminders with parent app payment link, 1Link/Raast QR on challans, bank statement matching, predictive default risk (AI) and proactive outreach, parent-facing family dues dashboard.

## Hands-on observations (live school tenant, 2026-10-12; no personal data copied)

Fee dashboard (month-scoped): collected vs expected (e.g. 18% of the month's challans), outstanding (all time, count of students), overdue (count of challans past due + amount), defaulters (30+ days overdue). Challan status counts: Generated (of N expected = enrolled students), Draft (pending approval), Paid (+ partial), Unpaid (released). "Not yet due" vs "Overdue" split of outstanding. Quick actions: Review Challans, Collect Fee, Payments Received, Advance Fee, Additional Fees (fines, exams, events), Fee History, Download Invoice, Print Challans (a class at a time), Needs Attention, Generate Fee (manual), Security Deposits, Fee Types, Fee Settings.

Fee Settings (5 areas): Automatic Generation (on: drafts created on the 1st for review, due on the 10th; a banner states the next scheduled run); Fee Challan layout template; Bank Accounts (which account the challan names, else "pay at the school office"); Invoice Settings (receipt number format PREFIX-YEAR-SEQUENCE, e.g. XXX-2026-000001); Online Payments (not set up here, so parents cannot pay online; separate gateway setup).

Fee Types: 14 types, default amount is only a suggestion, never a lock. Monthly Tuition is "set per class" (no default here). Behaviour per type: Monthly, On admission (Admission, Registration, Security Deposit refundable), or Charged as needed (Transport, Library, Sports, Laboratory, Examination, Event, Fine, Other...). Types can be switched OFF and the school can add its own. Each type shows a count of how many times it has been charged.

Review Challans: queue of drafts; empty when none. Approval releases them.

Collect Fee: list of outstanding students with filters (month, fee type incl. Remaining Dues, class, section) and tabs (All outstanding, Overdue, Partial, Defaulters 30d+, Family billing). Rows show student, class, number of fee lines, days overdue, parent name/mobile, amount, Collect button. Bulk selectors: all outstanding, overdue, defaulters. Export CSV/PDF. Family billing groups siblings into one family payment (one receipt covering several students and fee lines).

Payments Received: totals split by counter vs online, filters (date presets, method, recorded-by, class, section, show voided), table with paid-on time, student (or family with "+N" siblings and "N fee lines"), method, deposit-to account (e.g. Cash in Hand), recorded-by user, receipt number, amount. Payments can be voided (a "Show voided" filter exists).

Needs Attention: automatic checks page; flags failing payment numbers, bank money not recorded, accounting entries that failed to post. Empty = all good.

Takeaways for our spec: draft->review->release is real and used; receipt numbers are sequential per year; payments post to a cash/bank account in the ledger; family-level collection is a first-class mode; overdue/defaulter thresholds (30d) drive collection UX; online payment is a separate, opt-in setup (our gap).

## Hands-on addendum 2 (previously unloaded pages; read 2026-10-12)

Navigation quirk: Advance, Deposits, History, Generate pages render blank on a direct URL load but load when reached by clicking the link from the Fee hub (client-side navigation). First click after a page load is sometimes ignored; clicking twice works.

- **Additional Fees** (one-off charges, "applied instantly as Unpaid challans"): fee type chips (Fine, Examination, Event, Transport, Library, Sports, Lab, Admission, Registration, Security deposit, Remaining dues, Other); title/description*, amount*, due date (default +7 days), notes; apply to Student / Section / Class / All classes.
- **Advance Fee Collection**: 4 steps: 1 student, 2 month (this month up to 12 months ahead), 3 fee (shows what that month costs), 4 collect: amount received* (any part of it), payment date, payment method (Cash, Bank Transfer, Online, Cheque, Card), reference, notes. Rule shown on screen: the challan for that month is created if it does not exist, then the payment is recorded against it. There is no separate credit balance: advance = pay a future challan early.
- **Security Deposits**: tiles Held, Collected, Refunded, Still due (billed but unpaid); per student: collected, refunded, held, Refund button. Text: never counted as school income; sits on the balance sheet until returned. A built-in consistency check compares the deposit schedule with the ledger account and warns when they differ ("schedule shows 4,000 held but ledger carries 32,900; run gl:verify"): billed-but-unpaid deposits seem to be booked into the ledger liability. Lesson for us: book deposits only when received, and keep a sub-ledger-to-GL reconciliation check.
- **Fee History** (per student): standard fee, monthly fee after discount ("-PKR 2,000 discount"), outstanding with overdue count and days overdue, status (Overdue / All Clear), View History. Tiles: Outstanding (161), Defaulters (6), All Clear (47). The class standard fee and per-student discounted monthly fee are both stored (e.g. standard 5,000, monthly 3,000).
- **Generate Fee** (manual): month, year (2024-2027), due date; per student "Has Challan" or "Will Generate"; filters class/section; "N without challan, M already have challan"; generation is idempotent (only creates missing challans). Header repeats the automatic run time and the month it will draft.
- **Salary hub** (read): month snapshot, 14 active staff, nothing generated yet for October (this school does not use payroll in the system). Quick actions: Review Drafts, Pay Salary, Salary History, Salary Policies, Generate Salaries, Salary Settings. Generate/Pay/History pages not opened (click blocked by the automation permission check).

## Hands-on addendum 3
- **Print Challans**: a school setting decides the challan style. Here: "One challan per student, everything owed" (every unpaid fee across all months on one document; month and fee-type filters are not offered because the challan is the whole account); a Change link switches the style. Tiles: challans to print, students covered, overdue, total value; class/section filters; per-row list with covers ("Everything owed - N fees"), amount, due date, days; "Download all (N)" as one PDF.
- **Download Invoices**: every issued invoice; filters class, section, period (month list back 2 years), fee type, invoice type (Single/bulk payment vs Family collect or statement), source tab Automated / Custom; "Create custom invoice" button; download the filtered stack as one PDF. Tiles: invoices, students, family count, value.
