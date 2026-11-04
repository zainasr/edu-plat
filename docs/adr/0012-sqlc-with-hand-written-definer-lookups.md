# ADR-0012: sqlc for module queries, hand-written pgx for the two definer lookups

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-04 |
| Spec links | ADR-0004 (explicit SQL), ADR-0011 §3, §7 |

## Context
ADR-0011 chose sqlc for data access, so that every query is plain SQL that is typed at compile time. The two controlled cross-tenant reads (`resolve_api_token`, `tenant_id_by_slug`) are `SECURITY DEFINER` functions that return `TABLE(...)`. When sqlc parsed them, it could not resolve their output columns and rejected the queries (`column "id" does not exist`).

## Options considered
1. **Expose the functions as views, or wrap them in a sqlc query with an explicit cast**: sqlc still could not type the set-returning function, and a view would need owner-level grants that weaken the boundary.
2. **Drop the definer functions and use a policy-only lookup**: a token lookup needs a tenant before RLS can apply, so there is nothing to look up with. This is the problem the functions solve.
3. **Hand-written pgx for these two reads only**: the SQL is three lines, the call is explicit, and the column scan is checked by the compiler.

## Decision
Option 3. The two lookups live in `services/api/internal/iam/lookups.go`, outside sqlc. Everything else stays in sqlc. The lookups are the only tenant-less reads in the codebase, so they are kept in one reviewed file, and their SQL names the function explicitly.

## Consequences
- Good: the controlled boundary is visible in one place. The compiler checks the scan targets. The rest of the data layer is unchanged.
- Bad: two queries are not generated, so a change to the function's columns must be made in both the migration and `lookups.go`. The integration tests catch a mismatch.
- Follow-up: if sqlc gains support for table-returning functions, move these two into the generated package and delete `lookups.go`.
