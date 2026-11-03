# 02. Technology Stack and Delivery

| | |
|---|---|
| Status | Draft v0.1 (choices marked *Decide in design* are confirmed during the design phase) |
| Release | 1.0 |
| Module code | TECH |

## 1. Stack at a glance

| Layer | Choice | Why |
|---|---|---|
| Backend API | **Go**, standard library `net/http` routing, few mature libraries | Single static binary, low memory, simple operations, long-lived maintainability, owner learning goal |
| Database access | `pgx` with `sqlc` (type-safe SQL, no ORM) | Explicit SQL for money and ledger code |
| Migrations | `goose` or `atlas` *(Decide in design)* | Versioned, backward-compatible migrations |
| Background jobs | Postgres-backed queue (e.g. River) *(Decide in design)* | No extra broker at the start |
| Database | **PostgreSQL**, `tenant_id` + row-level security | Mature, transactional, PITR backups |
| Cache / rate limits | Redis | Sessions, rate limiting, ephemeral state |
| Files | DigitalOcean Spaces (S3-compatible) + CDN | Portable object storage |
| PDF generation | Headless Chromium renderer service (e.g. Gotenberg) | Go PDF libraries are weak; HTML templates are easy to brand |
| Web frontend | **Next.js** (TypeScript) | Default, mature ecosystem |
| Mobile | **React Native** (parent, teacher, student apps) with local database + sync *(sync library: Decide in design)* | One codebase, offline need |
| AI / ML | **Python** services, separate from the Go core | Ecosystem; isolates ML dependencies |
| Push | FCM | Standard on Android |
| Hosting | **DigitalOcean, Bangalore (India)**, self-managed | Cost, latency, owner skill |
| Containers | Docker; a few nodes behind a load balancer. Kubernetes is deliberately not used at the start | Team size |
| IaC | Terraform *(Decide in design)* | Reproducible infrastructure |
| CI/CD | GitHub Actions *(Decide in design)* | Tests, scans, image build, deploy |
| Observability | Self-hosted metrics, logs, traces, uptime checks | Cost; data stays with us |

## 2. API-first principle (DEC-004)

1. The **OpenAPI 3 specification** is the single source of truth. It is written/maintained in the repository and drives server stubs, typed clients (TypeScript for web and React Native), documentation and tests.
2. **Every product capability is an API endpoint.** The dashboard, mobile apps and any script use the same endpoints. There is no hidden admin API and no UI-only logic that cannot be reached by API.
3. Anything done in the portal (for example creating a student, releasing challans, recording a payment) can be done from a terminal with `curl` or similar and an **API token**, subject to the same permissions and audit.
4. Documentation is generated from the spec and published as an interactive API reference. Examples are `curl` first.
5. No separate CLI product is built. (A thin convenience wrapper may be added later only if it saves effort; the API is the product.)

Details: `02-cross-cutting/04-api-conventions.md`.

## 3. Architecture shape

```
Next.js web ──┐
RN apps ──────┼──> API (Go modular monolith) ──> PostgreSQL (tenant_id + RLS)
curl / scripts┘            │  │  │
                           │  │  └─> Redis (rate limit, sessions)
                           │  └────> Job workers (PDF, import, notifications, reports)
                           └───────> Event log ──> Analytics store ──> Python AI services
```

Modules (bounded contexts): `organisation`, `iam`, `students`, `admission`, `attendance`, `fees`, `accounting`, `staff`, `academics`, `behaviour`, `communication`, `analytics`, `platform`. Modules talk to each other through defined interfaces and internal events, not by reaching into each other's tables.

## 4. Environments and delivery

| Environment | Purpose |
|---|---|
| dev | Developer and agent work |
| staging | Pre-release verification |
| pilot | Production-like, for pilot schools |
| production | Customers |

- Trunk-based development, small pull requests, mandatory review (human + automated).
- Every PR runs unit, integration (real Postgres), contract (OpenAPI) tests, lint, dependency/container/IaC scans.
- Feature flags for incomplete features; blue-green or canary deploys; backward-compatible migrations; instant rollback for web; forced-update mechanism for breaking mobile API changes.
- No manual changes in production; everything via pipeline.

## 5. Availability and data protection on this infrastructure

- At least two application nodes behind a load balancer; PostgreSQL with a standby and tested failover; automated backups with point-in-time recovery; backups copied to a separate region/provider; quarterly restore drills.
- Targets are in `02-cross-cutting/06-non-functional-requirements.md`.

## 6. Learning plan note

The build doubles as a Go learning path for the product owner: fundamentals, concurrency, web and data, then product-level patterns (module structure, money handling, observability, graceful shutdown). A separate learning roadmap lives outside this specification.

## 7. Risks of this stack

| Risk | Mitigation |
|---|---|
| Few engineers in the market write Go | One engineer already does; keep code plain and conventional; strong tooling and review |
| Self-managed HA database with a small team | Start with a managed Postgres offering if self-run HA proves costly *(Decide in design)* |
| Types not shared between Go and TypeScript | Generate clients from OpenAPI |
| Offline sync complexity | Narrow scope (attendance, marks, diary drafts), idempotent operations, tested conflict rules |
| Payment aggregator onboarding delay | Start onboarding early; counter payments work without it |
| WhatsApp template approval delay | Start BSP onboarding early; in-app notifications are the fallback |
