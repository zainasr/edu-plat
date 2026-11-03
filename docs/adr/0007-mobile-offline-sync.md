# ADR-0007: React Native apps with local database and operation outbox

| | |
|---|---|
| Status | Proposed |
| Date | 2026-10-28 |
| Spec links | DEC-005, REQ-APP, OI sync library |

## Context
Teachers and parents have weak connectivity. Attendance, marks and diary drafts must work offline and sync safely.

## Options considered
1. **Local DB + outbox of idempotent operations**: client creates operations with IDs; server applies once.
2. **Full replication sync engine**: more automatic, heavy and a lock-in.
3. **Online-only**: simplest, fails the offline need.

## Decision
Start with option 1 for a narrow scope (attendance, marks, diary drafts). Library for the local DB is chosen when the first mobile slice starts.

## Consequences
- Good: small, testable conflict rules.
- Bad: custom sync code to maintain; keep the scope narrow.
