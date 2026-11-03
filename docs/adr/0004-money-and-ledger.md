# ADR-0004: Money as integer minor units with a hidden double-entry ledger

| | |
|---|---|
| Status | Accepted |
| Date | 2026-10-28 |
| Spec links | DEC-011, DEC-016, BR-FEE-008 |

## Context
Fees, receipts, salaries and expenses must always reconcile. Schools think in simple screens; auditors need a correct ledger.

## Options considered
1. **Integer minor units + append-only double-entry journal**: correct by construction, reversible, auditable.
2. **Balances stored on rows and updated in place**: simple, but drifts and cannot be audited.
3. **Floating point or decimals in app code**: rounding errors.

## Decision
Money is `int64` paisa in a small Money type. Every financial action writes a balanced journal entry in the same transaction. Receipts and journal rows are append-only; corrections are reversing entries; voided numbers are kept.

## Consequences
- Good: provable totals, safe corrections, trial balance on demand.
- Bad: more upfront modelling; reports read from the ledger.
- Follow-up: a test asserting debits equal credits after every financial command.
