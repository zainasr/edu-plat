# ADR-0002: Multi-tenancy with shared Postgres, tenant_id and row-level security

| | |
|---|---|
| Status | Accepted |
| Date | 2026-10-28 |
| Spec links | DEC-002, REQ-NFR security |

## Context
Many schools share the platform; one school must never see another's data. Schools are small, so per-tenant databases are too costly.


## Options considered
1. **Shared schema + tenant_id + RLS**: cheapest to run, isolation enforced by the database as a second line of defence.
2. **Schema per tenant**: stronger separation, painful migrations at hundreds of tenants.
3. **Database per tenant**: strongest isolation, highest cost and operational load.

## Decision
Shared schema. Every tenant table has `tenant_id` and an RLS policy driven by a per-transaction setting; the app connects with a role that cannot bypass RLS. Cell-based deployment is the later scale path.

## Consequences
- Good: low cost, simple migrations, isolation enforced below the application.
- Bad: every query must run in a tenant-scoped transaction; cross-tenant admin work needs a controlled path.
- Follow-up: integration tests that prove a second tenant cannot read or write the first tenant's rows.
