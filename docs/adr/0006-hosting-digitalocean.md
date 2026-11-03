# ADR-0006: Self-managed hosting on DigitalOcean (Bangalore) with Docker and blue-green deploys

| | |
|---|---|
| Status | Accepted |
| Date | 2026-10-28 |
| Spec links | DEC-007, REQ-NFR availability |

## Context
Owner has DigitalOcean servers and strong DevOps skill; budget is modest; target is high availability for fee collection. No Kubernetes expertise or need yet.

## Options considered
1. **Docker on a few droplets + load balancer, Terraform, GitHub Actions, blue-green**: simple and reproducible.
2. **Kubernetes**: flexible, heavy for the team.
3. **PaaS**: easiest, less control and cost predictability.

## Decision
Option 1. Two or more app nodes behind a load balancer, PostgreSQL with a standby and PITR, backups copied to a separate region, deploys only via pipeline. Managed Postgres is allowed if self-run HA proves costly.

## Consequences
- Good: control, low cost, matches owner skills.
- Bad: we own failover and patching.
- Follow-up: quarterly restore drill; runbooks in `docs/runbooks/`.
