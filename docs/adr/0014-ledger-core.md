# ADR-0014: Ledger core: balance enforced by the database, gapless numbers, internal-only posting

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-07 |
| Spec links | DEC-011, DEC-016, BR-FEE-002, BR-FEE-003, BR-FEE-004, BR-FEE-008, REQ-FEE-024, ADR-0004 |

## Context
ADR-0004 decided that money is integer paisa and that every financial action posts a balanced, append-only journal entry. This ADR records how the first slice makes that true and provable. Fees, receipts, refunds and payroll will all post here, so a mistake in this layer corrupts every later feature.

## Options considered

**Where "debits equal credits" is enforced**
1. **Only in Go.** Simple, but any bug, any raw SQL, or any future module that forgets the check can commit an unbalanced entry.
2. **In Go (friendly error) and in the database (guarantee).** A deferred constraint trigger checks each entry when the transaction commits.

**Entry numbers**
1. **A Postgres sequence.** Fast, but sequences never roll back, so a failed transaction leaves a hole. Receipt-style numbering needs no gaps (BR-FEE-002).
2. **A per-school counter row updated in the same transaction** (the pattern already used for registration numbers). A rollback gives the number back; concurrent posts queue on one row lock.

**Who can post**
1. **A public `POST /journal-entries`.** Flexible, but it hands a bookkeeping tool to every caller before approval rules and period close exist.
2. **An internal engine only, with a read-only API.** Fee and accounting code post inside their own transaction; clients read.

**The chart of accounts**
1. **Fully configurable now.** More endpoints and rules before any fee uses them.
2. **A fixed built-in chart, created per school on first use** (like the default branch in ADR-0013). Custom accounts come with accounting.

## Decision
- **Balance** is enforced twice: `validateEntry` in Go returns a clear error, and migration `00005` adds `CONSTRAINT TRIGGER ... DEFERRABLE INITIALLY DEFERRED` on `journal_entries` and `journal_lines`. At commit it requires at least two lines and equal debit and credit totals (SQLSTATE 23514). Lines also carry `CHECK` constraints: non-negative, and exactly one side above zero.
- **Entry numbers** come from the school's `tenant_counters` row `journal-entry`, inside the posting transaction. Posting takes the number last, after every cheap check.
- **Append-only** for every role: no `UPDATE` or `DELETE` grant to the API role, and a `forbid_mutation` trigger that also stops the owner. `accounts` cannot be deleted.
- **Corrections** are reversing entries. `Reverse` posts the mirror image linked by `reverses_entry_id`. An entry can be reversed once (partial unique index), and a reversal cannot be reversed.
- **Posting is internal**: `ledger.Service.Post(ctx, tx, principal, entry)` takes the caller's open tenant transaction and never begins or commits, so a business change and its entry commit together. The HTTP API is read-only: accounts, journal entries, trial balance.
- **Chart**: seven built-in accounts (cash in hand, bank, fee receivable, advance fees received, security deposits held, fee income, discounts given), created by one `INSERT ... ON CONFLICT DO NOTHING`. Security deposits are a liability, never income (BR-FEE-004). The Go constants and the SQL list are checked against each other by a test.
- **Money** is `money.Money` (an `int64` of paisa) with an overflow-checked `Add`. The API sends `{amount, currency}` with an integer amount.

## Consequences
- Good: the books cannot be unbalanced by application bugs or hand-written SQL; numbers have no gaps; history cannot be edited; a reversal restores every account to its earlier totals.
- Bad: concurrent posts for one school serialise on the counter row (acceptable at school scale; revisit if posting rates grow). The deferred trigger runs per line, so a 200-line entry runs it 200 times (negligible, bounded by `maxLines`). The built-in chart cannot be customised yet.
- Verified: with the balance triggers deliberately disabled, the test `TestADR_0004_DatabaseRefusesAnUnbalancedEntryAtCommit` fails on all three of its cases, so it detects a broken guarantee.
- Follow-ups: fiscal periods and period close; custom accounts (`account.manage`); a manual journal endpoint behind `journal.manual` and approval; per-year receipt numbering (REQ-FEE-024) built on the same counter idea.
