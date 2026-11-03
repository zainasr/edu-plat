# 16 Expense Management
Routes: /admin/expenses, /expenses/create, /expenses/categories. Screenshots: screenshots/16-expenses/.
- List: tiles (expenses, categories, page total, average per expense), search by title/description, category filter, empty state. Buttons Categories, Add Expense.
- **Add expense** form: Category (optional, link to manage categories), Title*, Amount (PKR)*, Date spent* ("when the money actually left", accrual hint), Paid to (vendor, optional; used to group spend by vendor in expense report), **Paid by** (decides which cash or bank account it comes out of), **Settlement: Unpaid - on credit** (records a payable instead of money out), Description.
- Categories: simple list (name), tiles: configured, in use, categorized expenses. None preconfigured.
- Accounting link: paid-by account and credit option imply double-entry postings (see 17: ledger, trial balance, accounts, payables).
- Entities: Expense(category_id, title, amount, date, vendor, paid_from_account_id, is_credit, description), ExpenseCategory(name).
- Gaps/ideas: no receipt attachment, recurring expenses (rent, electricity), approval workflow, budget per category (budget report exists), petty-cash handling. Ideas: photo-of-bill capture with OCR on mobile, recurring templates, expense anomaly alerts.
