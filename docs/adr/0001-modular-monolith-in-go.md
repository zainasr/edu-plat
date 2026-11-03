# ADR-0001: Modular monolith in Go with the standard library

| | |
|---|---|
| Status | Accepted |
| Date | 2026-10-28 |
| Spec links | DEC-003, DEC-005 |

## Context
Three to four engineers plus AI agents must ship a school ERP with money, attendance and messaging, run on a few DigitalOcean nodes. The owner wants simplicity, long-term maintainability and to learn Go and standarad software engineering princples so the choice is only golang(good mixture of both low level + modern stuff + simple).

## Options considered
1. **Go modular monolith, standard library HTTP**: one binary, simple deploys, fast, explicit code that agents read well. Fewer batteries than a framework.
2. **Node/TypeScript backend**: one language with web and mobile; weaker for long-lived, CPU-light concurrent services and for the owner's learning goal.
3. **Microservices**: independent scaling but heavy operations for this team.

## Decision
Go modular monolith. One package per bounded context under `services/api/internal/`, talking through exported interfaces and internal events. Python is used only for the separate AI service.

## Consequences
- Good: one deployable, simple local dev and CI, easy refactor to services later.
- Bad: discipline is needed to keep modules from reaching into each other (review checklist and the reviewer agent cover it).
- Follow-up: split a module into a service only when scale or team ownership demands it.
