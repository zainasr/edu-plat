# 01. Decision Log

| | |
|---|---|
| Status | Draft v0.1 |
| Release | All |
| Module code | DEC |

Each decision has a number, the choice, the reason and the owner. Changes are made by adding a new decision that supersedes the old one.

| ID | Decision | Reason | Date |
|---|---|---|---|
| DEC-001 | Follow the reference product's proven workflows wherever they work (fee draft-review-release, family billing, hidden double-entry engine, period close, audit log). Differ on technology, hosting, mobile, messaging and security. | The reference is mature in Pakistani school operations. | 2026-10-12 |
| DEC-002 | Multi-tenant SaaS. Tenant = Organisation. Shared database with strict `tenant_id` enforcement and row-level security; cell-based deployment for scale later. | Simplicity now, scalable path later. | 2026-10-19 |
| DEC-003 | Modular monolith with bounded contexts (students, fees, attendance, academics, comms, accounting, platform) and an internal event bus. Split services only when needed. | Small team; avoids distributed-system cost. | 2026-10-19 |
| DEC-004 | **API-first.** OpenAPI is the source of truth. Every feature is an API endpoint; web and mobile use the same public API; no UI-only paths. Anything doable in the portal is doable from a terminal with an API token. No separate CLI product. | Controllability, automation, agent-friendliness, testability. | 2026-10-19 |
| DEC-005 | Backend in Go (standard library HTTP + a few mature libraries). Python only for AI/ML services. Web in Next.js. Mobile in React Native. Database PostgreSQL. | Owner skill and learning goal, operational simplicity, strong fit for a small team. | 2026-10-19 |
| DEC-006 | **A user may hold many roles at once in one institution; effective permissions are the union of all roles and scopes.** No role switching inside an institution. Maker-checker prevents one person approving their own action. | Pakistani institutions combine duties in one person. | 2026-10-19 |
| DEC-007 | Hosting: self-managed on DigitalOcean, India (Bangalore) region; Docker, infrastructure as code, CI/CD, blue-green deploys, backups to a separate region/provider. | Cost control, low latency to Pakistan, owner DevOps skill. | 2026-10-19 |
| DEC-008 | Payments through ONE Pakistani aggregator exposing Easypaisa, JazzCash, bank/Raast and cards; funds settle to the school. We do not hold school money. Online payment is optional per school. | Avoid becoming a payment institution; school adoption. | 2026-10-19 |
| DEC-009 | Notification strategy: in-app/push first; WhatsApp (official API through a BSP) for important reminders only; SMS only as OTP/critical fallback. WhatsApp is paid per template message outside the 24-hour reply window. | Cost and reliability. | 2026-10-19 |
| DEC-010 | Advance/overpayment follows the reference: payment against a future challan; no free-floating family credit balance. Unallocated money must be applied or refunded explicitly. | Simplicity; proven. | 2026-10-19 |
| DEC-011 | Never hard-delete business records. Deactivate, version, reverse. Receipts and journal entries are append-only; corrections are reversing entries. | Audit and trust in money data. | 2026-10-12 |
| DEC-012 | AI scope: rule-based, transparent insights in 1.0; capture an immutable event log from day one so ML can follow. Further AI scope is decided as we build. | Moat needs data, not early models. | 2026-10-19 |
| DEC-013 | Pricing and plan tiers are decided later. The data model supports feature flags, limits and plans from day one. | Needs pilot feedback. | 2026-10-19 |
| DEC-014 | Platform admin console is not needed on day one; operate through the API and minimal tooling, then build the console as the product grows. | Focus on school value. | 2026-10-19 |
| DEC-015 | UI language is English only. Content fields support Urdu (Unicode, RTL rendering). All UI strings are externalised so Urdu can come later. | Scope control; readiness. | 2026-10-12 |
| DEC-016 | Build a hidden double-entry posting engine from day one; expose plain-language screens first. | Correct money from day one, simple UI. | 2026-10-12 |
| DEC-017 | Brand/product name is decided later; the name is configuration, never hard-coded. | Not blocking. | 2026-10-19 |
| DEC-018 | Budget, team size growth, legal entity, trademarks and store accounts are handled on the go and tracked as open items. | Owner direction. | 2026-10-19 |
| DEC-019 | Development process: phased gates (requirements, design, validation, planning, build, test, release) with agile sprints inside build. The spec is revisable; change after sign-off goes through change requests. | Keeps flexibility with control. | 2026-10-19 |
| DEC-020 | Attendance is school-controlled and phone-independent: web scan page in the normal portal on the reception computer (webcam reads the ID card QR; no extra device or kiosk), absentees-only class register on school devices, attendance-desk entry from paper slips, and an optional teacher app. Absent alerts only from submitted registers. | Teacher phones are banned in many schools. | 2026-10-21 |
| DEC-021 | Diary is lecture-based: teachers plan per section-subject-date in advance, confirm delivery by exception, and the system sends one combined class diary to parents near school end, with an Admin coverage view. | Matches how schools actually work. | 2026-10-21 |
| DEC-022 | A guardian is any responsible adult; the father is not required to be a guardian. Father's name stays as an identity field. Each guardian-student link carries a relationship and flags (primary, fee payer, notifications, emergency, access). The family page has a tree view of guardians and students. | Common in Pakistan: children raised by uncles, grandparents or relatives. | 2026-10-27 |
| DEC-023 | The guardian is the person who pays the fees; original parents who do not pay need not be linked. | Real family situations in Pakistan. | 2026-10-27 |
| DEC-024 | Admission starts with an enquiry and a branded information pack (PDF) containing a per-student customised fee quotation, followed by individual office follow-up and conversion; accepted quotation amounts carry into the admission challan. | Matches how schools sell admissions. | 2026-10-27 |
