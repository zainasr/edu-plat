# 17 Accounting & Financial Reports
Routes: /admin/reports and /reports/{dashboard, unit-economics, profit-loss, balance-sheet, cash-flow, equity, comparison, budget, bridge, trend, receivables, defaulters, fee-collections, collections, concessions, payroll, expenses, assets, trial-balance, ledger, journal, periods, accounts}. Screenshots: screenshots/17-reports/. Opened and read: index, owner dashboard, P&L, defaulters, chart of accounts, period close. The rest are described from the index page only.

## Index (grouped, "Fiscal year 2026 · 380 ledger entries")
1. Start here: **Owner Dashboard** (12 headline figures with threshold alerts, tiles link to detail), **Unit Economics** (revenue/cost per student, class contribution, break-even, cash runway, fee coverage).
2. Financial statements: Profit & Loss (cash or accrual), Balance Sheet, Cash Flow (reconciled to bank), Changes in Equity.
3. Comparison & planning: Comparison (MoM, QoQ, YoY by line), Budget vs Actual (set year plan), Bridge Analysis (why revenue/cost moved: enrolment, rate, mix, concessions), Seasonality & Trend (rolling 12 months).
4. Receivables & collections: Receivables Aging (bucketed, by class or family), Defaulters, Fee Collections (by day/week/month/quarter/year, fee heads, classes), Collections (efficiency, DSO, cash book, recovery curve, write-offs), Concessions & Yield (discounts given, share of list price).
5. Payroll & operations: Payroll (register, cost by department, deductions, security deposits, attendance leakage), Expense Analysis (by category and vendor, largest items), Fixed Assets (register, net book value, monthly depreciation run).
6. Ledger & controls: Trial Balance, General Ledger (one account, running balance), Journal (full audit trail, manual entries), Period Close, Chart of Accounts.

## Owner Dashboard details
FY selector (FY2021-FY2027), CSV/PDF/print. "Needs attention" alert box (e.g. "Collection is behind: 17.75% collected vs 65% floor" with link to defaulters). Tiles: net revenue, net profit, operating margin, cash on hand (+ current ratio), cash runway (months at average monthly burn), outstanding fees (students), over-90-days share of receivable book, collected this month, collection efficiency YTD, payroll ratio, revenue per student, break-even enrolment. Compared with last year.

## P&L
Basis toggle Accrual/Cash; period Month/Quarter/Fiscal year/Custom; comparison none / prior period / prior year; CSV/PDF/print. Accounts with codes and "view ledger" drilldown: Tuition, Admission, Library, Sports, Laboratory, Examination, Fine, Other, Remaining Dues income; concessions shown as contra revenue (4900); net revenue; expenses; net profit and margin. Fiscal year is calendar year here (Jan-Dec), whereas Pakistani schools typically run on academic year/July-June or April-March, so confirm.

## Defaulters & Collections Worklist
Filters: as-at date, Students/Families/In credit, sort (largest balance, longest overdue, name), owes at least, overdue by N days, include left students; CSV/PDF/print. Tiles: total outstanding, students owing, average balance. Aging columns per student: Current, 1-30, 31-60, 61-90, 90+, total; last payment date/amount; family number.

## Chart of Accounts (double-entry engine)
Fixed system chart with codes: Assets (1000 Cash in Hand, 1100 Fee Receivable, 1200 Prepaid, 1500 series Fixed Assets - land & building, furniture, IT, vehicles, library books, 1590 Accum. depreciation), Liabilities (2000 Salary payable, 2010 Staff security deposits held, 2020 statutory deductions payable, 2030 student security deposits held, 2100 Unearned fee income, 2200 Accounts payable, 2500 Loans), Equity (3000 Capital, 3100 Drawings, 3900 Retained earnings), Income (4000 series per fee head, 4900 discounts & concessions, 4950 other), Expenses (5000 salaries by staff type 5001-5009, 5900 depreciation, 5950 bad debt/write-off). Codes fixed, names renameable, unused accounts hide-able; accounts auto-appear when used. "Cash & bank" accounts are added by hand (default Cash in Hand; payments name the account). **Opening balances** can be set when the school was running before (else balance sheet starts at zero).
Every fee/salary/expense action auto-posts journal entries (380 entries so far); the "Needs attention" fee page watches for failed postings.

## Period Close
Per month Open / Close; "Months closed 0 of 12"; closed month refuses postings (they roll forward to the next open period, original date kept in memo); current month cannot be closed.

## Observations / ideas
- This is the most differentiated part of the reference: a real accounting core hidden behind simple fee actions. Good fit for "accounting and reports" and "finance management" MVP modules.
- Risks for our market: heavy for non-accountants (many reports may overwhelm), so we should lead with a plain-language owner view and an AI "what should I do this week" summary.
- Gaps: no bank reconciliation import, no tax/FBR/EOBI, no multi-branch consolidation, no inventory/fixed-asset UI verified, fiscal year not configurable here. Ideas: explain-this-number AI, cash-flow forecast, fee-default prediction, auto budget suggestions, WhatsApp-able owner daily summary.
