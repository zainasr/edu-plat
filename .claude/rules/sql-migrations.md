---
paths:
  - "services/api/migrations/**"
  - "services/api/**/*.sql"
---
# SQL and migrations
- Forward-only, backward-compatible: expand, migrate, contract across releases. No destructive change in the same release that stops using the column.
- Every tenant table: `tenant_id uuid not null`, an index starting with `tenant_id`, `enable row level security`, and a policy using the session tenant setting.
- Primary keys are UUIDs. Timestamps are `timestamptz`. Money columns are `bigint` minor units.
- Ledger tables (journal entries, receipts) are append-only: no UPDATE/DELETE grants; corrections are reversing rows.
- sqlc query files are named after the module; one query per business need, explicit column lists (no `select *`).
