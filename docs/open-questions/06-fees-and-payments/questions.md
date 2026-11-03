# 06 Fees and payments

1. (BLOCKER) Fee structure model: class-level monthly fee + per-student override/discount (reference). Do we also need fee plans per class per head (tuition, transport, lab), annual/term/one-time fees, and per-subject fees for academies?
2. Billing cycle: monthly challans only, or also quarterly/term/annual billing options per institution or per student?
3. Challan generation: auto-draft on day X, review/approve, release (reference). Keep? Who approves? Bulk approve? Can parents see drafts? What day/due-date defaults?
4. Due date, grace and late fee: fixed late fine, per-day fine, percentage? Cap? Waiver flow with approval? Auto-applied or manual?
5. Discounts and concessions: types (sibling, staff child, merit, need-based, scholarship), fixed vs %, duration (permanent/one session/one month), approval and reporting. Which types in 1.0?
6. Partial payments and allocation: how is a payment allocated across multiple dues (oldest first, tuition first, user-chosen)? Advance payments and credit balance handling? Refunds?
7. Arrears and opening balances at migration: handled as "remaining dues" like reference?
8. Collection channels in 1.0: cash at counter, bank deposit (challan with bank slip), cheque, JazzCash, Easypaisa, 1Link/Raast, card. Which are in 1.0 vs later? Who is the settlement owner (school's own merchant account or our aggregated account)?
9. Reconciliation: how do bank/wallet payments get matched to challans (unique challan/consumer number, 1Bill/1Link voucher, bank file upload, API/webhook)?
10. Receipts and invoices: numbering rules, formats (A4, A5, 80 mm thermal), reprint, void with reason and approval, family receipt vs per-student?
11. Defaulters and recovery: rules for reminders (day -3, due day, +3, +7, +15), escalation, blocking result card/exams/app features for unpaid fees (policy options), struck-off rule?
12. Security deposits and refundable fees: hold, adjust against last dues, refund approval, ledger treatment.
13. Fee structure changes mid-year: increase fees mid-session, effective-from date, recalculating unreleased drafts only?
14. Multiple children in different branches/shifts: one family statement and one payment for all?
15. Fee heads that are not income (donations, fund, books/uniform sales): needed? Inventory/uniform shop in scope?
16. Parent-facing fee experience: what the parent sees (dues, history, receipts, payment methods), pay-all vs pay-one-child, payment confirmation flow.
17. Tax/regulatory: withholding tax on fees, receipts needing NTN/FBR, any expectations?
