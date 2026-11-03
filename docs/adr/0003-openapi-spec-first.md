# ADR-0003: OpenAPI spec-first with generated server types and clients

| | |
|---|---|
| Status | Proposed |
| Date | 2026-10-28 |
| Spec links | DEC-004, REQ-API-001..005 |

## Context
Every capability must be an API endpoint usable from portal, mobile and terminal. Web and React Native need typed clients; the Go server needs types that match the contract.

## Options considered
1. **Spec-first**: write `openapi.yaml`, generate Go server types/handlers (e.g. oapi-codegen) and TypeScript clients. Contract cannot drift.
2. **Code-first**: annotate Go handlers and generate the spec. Faster at first, drifts easily and hides the contract.
3. **Hand-written client and server**: no tooling, guaranteed drift.

## Decision
Spec-first. `api/openapi/openapi.yaml` is edited before code; generated output lives in `gen/` folders and is never hand-edited (a hook enforces this). Exact generator is confirmed when the first module is built and recorded by amending this ADR.

## Consequences
- Good: contract-first, typed clients, docs for free, easy contract tests.
- Bad: generator quirks and a build step (`make gen`).
- Follow-up: CI fails if generated code is stale.
