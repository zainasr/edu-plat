# Module 05. Accounting, Finance and Reports

| | |
|---|---|
| Status | Draft v0.1 |
| Release | 1.0 (advanced views and several reports in 1.1) |
| Module code | ACC |

## 1. Purpose
Correct books without bookkeeping effort. A double-entry posting engine runs from day one and is hidden behind plain-language screens for owners and clerks. Accountants may open the advanced area.

## 2. Requirements

### Engine
- **REQ-ACC-001** Every financial event (challan release, payment, void, refund, expense, salary, advance, deposit) posts a balanced journal entry automatically. A "Needs attention" list shows entries that failed to post.
- **REQ-ACC-002** Chart of accounts: fixed system chart (codes locked, names renameable); custom sub-accounts under expenses and income; cash/bank accounts added by an Admin; a cash book per branch.
- **REQ-ACC-003** Fiscal year configurable per institution (default July to June); reports can also filter by academic session.
- **REQ-ACC-004** Owner views default to **cash basis**; accountants can toggle accrual.
- **REQ-ACC-005** **Period close:** the Owner/Accountant can freeze a month; postings to a closed month are blocked and roll into an adjustment in the next open month. Manual journals only for the Accountant role, always audited. (ref)
- **REQ-ACC-006** Journals are append-only; corrections via reversing entries.
- **REQ-ACC-007** Per-branch books with a consolidated owner view; inter-branch transfers are recorded as simple transfers (no eliminations).

### Operations
- **REQ-ACC-010** Expenses: category, vendor (optional text), date, amount, paid now or on credit with later settlement, paid-from account, attachment (bill photo), single-level approval above a threshold. Recurring templates and petty cash are [1.1].
- **REQ-ACC-011** "Other income" generic entry with category. Dedicated donations/canteen/rental modules are deferred.
- **REQ-ACC-012** Cash sessions: opening, collections, closing count, variance (note required), day-close report; deposit to bank is a transfer between cash and bank accounts.
- **REQ-ACC-013** Bank entries are manual with reference in 1.0; statement import and auto-match in 1.1.
- **REQ-ACC-014** Opening balances wizard: dues per student/family, cash and bank balances, staff advances; posted as dated opening entries at go-live; locked after owner confirmation with a reconciliation summary. (ref)

### Reports
- **REQ-ACC-020** All reports export to Excel and PDF. No third-party accounting connectors in 1.0.
- **REQ-ACC-021** 1.0 report catalogue (grouped as in the reference):

| Group | Reports |
|---|---|
| Start here | Owner Dashboard |
| Financial statements | Profit & Loss (cash or accrual), Balance Sheet, Cash Flow |
| Receivables and collections | Receivables Aging, Defaulters, Fee Collections, Collections, Concessions & Yield |
| Payroll and operations | Payroll summary, Expense Analysis |
| Ledger and controls | Trial Balance, General Ledger, Journal, Period Close, Chart of Accounts |

  [1.1]: Comparison, Changes in Equity, Budget vs Actual, Bridge Analysis, Seasonality & Trend, Unit Economics, Fixed Assets (chart slots reserved). Budgets [1.2].
- **REQ-ACC-022** Scheduled reports (daily/weekly/monthly) can be sent to email/WhatsApp as PDF/Excel for the Owner/Accountant.
- **REQ-ACC-023** The Owner Dashboard leads with: collection % this month, defaulters, cash position, today's attendance, enrolment trend, staff cost, items needing action.

## 3. User stories

### US-ACC-001 Owner sees the numbers
- Given I am Owner, when I open the dashboard, then I see this month's collected vs expected fees, defaulter count, cash/bank balances and a list of actions (for example "Salaries not generated").

### US-ACC-002 Record an expense
- Given I enter "Electricity, Rs 18,500, paid from Cash in Hand, bill photo", when I save, then the cash balance decreases, an expense journal is posted and it appears in Expense Analysis.
- Given the amount exceeds the approval threshold, then it waits for an approver.

### US-ACC-003 Close the day
- Given I collected Rs 85,000 cash and counted Rs 84,800, when I close my cash session, then I must enter a variance note and the day-close report shows the 200 difference.

### US-ACC-004 Close the month
- Given October is reported, when the Owner closes it, then a backdated payment dated October is posted into November as an adjustment and the October reports do not change.

### US-ACC-005 Opening balances at go-live
- Given a school starts mid-year, when I enter opening dues for 120 students and cash Rs 50,000, then reports show the opening position and dues appear on challans.

## 4. Business rules
- **BR-ACC-001** Journal entries must balance (debits = credits).
- **BR-ACC-002** Closed periods cannot be edited.
- **BR-ACC-003** Deposits held are liabilities; they never appear as income.
- **BR-ACC-004** Accrual and cash views come from the same ledger; changing the view never changes data.
- **BR-ACC-005** Tax calculations are not automated in 1.0.

## 5. API (representative)
`/expenses`, `/expense-categories`, `/cash-sessions`, `/accounts`, `/journal-entries`, `POST /periods/{yyyy-mm}/close`, `GET /reports/{name}?basis=cash&format=pdf`.

## 6. Decide in design
Posting rules table per event; exact chart codes; review by a Pakistani chartered accountant for trust/non-profit schools, tax treatment and audit formats.
