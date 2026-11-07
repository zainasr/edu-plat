# Architecture Decision Records

An ADR records one significant technical choice: the context, the options, the decision and its consequences. The product spec says *what* and *why for the business*; ADRs say *how, and why this way*. Create one with the `/adr` skill. Never edit an accepted decision's meaning: supersede it with a new ADR.

| ADR | Title | Status |
|---|---|---|
| 0001 | Modular monolith in Go with the standard library | Accepted |
| 0002 | Multi-tenancy with shared Postgres, tenant_id and RLS | Accepted |
| 0003 | OpenAPI spec-first with generated server types and clients | Accepted (generator amended 2026-11-03) |
| 0004 | Money as integer minor units with a hidden double-entry ledger | Accepted |
| 0005 | Background jobs on a Postgres-backed queue | Proposed |
| 0006 | Self-managed hosting on DigitalOcean (Bangalore) | Accepted |
| 0007 | React Native apps with local database and operation outbox | Proposed |
| 0008 | Repository designed for AI-agent-assisted development | Accepted |
| 0009 | Migrations with goose and pinned developer tools | Accepted |
| 0010 | Postgres MCP server runs from a pinned Docker image | Accepted |
| 0011 | Walking-skeleton auth, tenancy and data access | Accepted |
| 0012 | sqlc for module queries, hand-written pgx for the definer lookups | Accepted |
| 0013 | Academic structure, the default branch, and cross-module calls inside one transaction | Accepted |
| 0014 | Ledger core: balance enforced by the database, gapless numbers, internal-only posting | Accepted |

Still to write when reached: token format and sessions, PDF renderer, observability stack, Terraform layout.
