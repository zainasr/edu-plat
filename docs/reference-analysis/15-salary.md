# 15 Salary (payroll)
Routes: /admin/salaries, /drafts, /pay, /history, /policies, /generate, /settings. Screenshots: screenshots/15-salary/.

## Hub
Month snapshot (October 2026): Paid, Outstanding (staff), Unpaid records, Active staff; payment progress bar; salary status counts (Generated of N staff, Draft pending approval, Paid this month, Unpaid approved); all-time summary and "staff waiting". Primary action **Generate October Salaries**. Quick actions: Review Drafts (edit & approve), Pay Salary (record payments), Salary History (per-staff records), Salary Policies (deductions & allowances), Generate Salaries (create draft records), Salary Settings.
Dashboard shows an action item when salaries for the month are not yet generated.

## Lifecycle (mirrors fees)
Generate -> **Draft** -> admin edits/approves -> **Unpaid (approved)** -> Paid (partial possible: "not fully paid"). Settings: monthly auto-generation toggle (off by default; drafts always need review).

## Policies (tabs)
- **Security deductions**: withheld from new staff for a set duration after joining, returned when they leave; only one active policy. (Uses staff joining date.)
- **Deduction policies** and **Allowance policies**: applied at generation time (contents empty on this account; likely late/absent/unpaid-leave deductions and fixed/percentage allowances).
- /generate and /history did not load in session (spinner); /pay not rendered.

## Entities
SalaryRecord(staff_id, period, base, allowances[], deductions[], net, status, approved_by), SalaryPayment(record_id, method, date, account), SecurityDeductionPolicy, DeductionPolicy, AllowancePolicy, link to staff attendance and leave for deductions.

## Gaps/ideas
No payslip view/PDF, bank transfer file (Pakistani bank bulk format), tax (income tax slab), EOBI/provident fund, advance/loan to staff, overtime or per-lecture pay (common in academies). Ideas: attendance-driven auto-deductions, per-lecture teacher pay for coaching centers, WhatsApp payslip.

## Hands-on addendum (2026-10-12)
- **Generate Salaries**: month, year, staff-type filter (Teacher, Manager, Administrative, Support, Security, Maintenance, IT, Finance, Other); per staff a fixed monthly salary and a status ("Will Generate" or already generated); "N without salary, M already generated" (idempotent, same pattern as fee generation); button Generate Drafts. 13 of 14 staff had a salary configured (fixed monthly amounts, e.g. 2,000-15,000); salary is one monthly figure per staff on the staff record, plus policy-based allowances/deductions.
- This school has not generated any salary yet (hub shows 0 of 14), so Review Drafts / Pay Salary / History were empty or blocked; payslip and bank-file gaps in the earlier notes stand.
