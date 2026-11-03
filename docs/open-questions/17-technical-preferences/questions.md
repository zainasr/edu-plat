# 17 Technical preferences and constraints (input to the design phase)

We decide architecture and stack in the design phase, but your constraints shape it. Answer what you already know.

1. (BLOCKER) Team: who builds (you + AI agents only, in-house team, outsourced)? Size, skills (backend, mobile, frontend, DevOps, data/ML), and budget for infrastructure per month in year 1.
2. Languages/frameworks: any hard preferences or bans? (e.g. TypeScript everywhere, Python for ML, Java/Kotlin, Go, PHP/Laravel, Flutter/React Native). Known strengths of your team matter more than fashion.
3. Mobile: native vs cross-platform (Flutter/React Native/Kotlin Multiplatform) preference? OTA updates needed?
4. Architecture appetite: modular monolith first (recommended for speed and simplicity) vs microservices from day one? Must the design allow splitting later?
5. Cloud: provider preference (AWS, GCP, Azure, DigitalOcean, local Pakistani hosting), credits/discounts available, region constraints (see section 15).
6. Database: relational (PostgreSQL) primary assumed; any constraints (managed vs self-hosted, MySQL familiarity)? Need for analytics/warehouse and search engine?
7. Multi-tenancy implementation: row-level tenant_id vs schema vs database per tenant (see section 15).
8. API style: REST, GraphQL, or both? Public API versioning approach, OpenAPI contracts generated first?
9. Real-time needs: chat, live attendance dashboard, notifications: WebSocket/SSE/Firebase?
10. Offline sync approach for mobile: local DB + sync (SQLite/WatermelonDB/PowerSync etc.) acceptable?
11. DevOps expectations: CI/CD, environments (dev/staging/prod/pilot), infra as code, containers/Kubernetes vs simpler PaaS, feature flags, blue/green or canary deploys?
12. Observability and security tooling expectations (see 14 Q15, 15).
13. Code quality bar: testing pyramid targets (unit/integration/e2e), coverage thresholds, code review rules, linting, architecture decision records (ADR), documentation standards.
14. Spec-driven development: which format (OpenAPI, Gherkin/BDD acceptance criteria, ER diagrams, state machines, event catalogue) and tool chain do you want for AI agents to consume?
15. AI agent workflow: which agents (Claude Code etc.), how do we split work (per module/vertical slice), review and merge policy, how human verification happens?
16. Open-source vs proprietary components, licensing constraints (e.g. avoid AGPL)?
17. Data/ML infrastructure: where do we train/serve (managed ML vs own), GPU budget, privacy constraints on cloud ML?
18. Vendor lock-in tolerance: managed services (Firebase, Supabase, Auth0) vs self-assembled?
