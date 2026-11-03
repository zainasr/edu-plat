# ADR-0003: OpenAPI spec-first with generated server types and clients

| | |
|---|---|
| Status | Accepted (generator chosen 2026-11-03 by spike; see Decision) |
| Date | 2026-10-28 |
| Spec links | DEC-004, REQ-API-001..005 |

## Context
Every capability must be an API endpoint usable from portal, mobile and terminal. Web and React Native need typed clients; the Go server needs types that match the contract.

## Options considered
1. **Spec-first**: write `openapi.yaml`, generate Go server types/handlers (e.g. oapi-codegen) and TypeScript clients. Contract cannot drift.
2. **Code-first**: annotate Go handlers and generate the spec. Faster at first, drifts easily and hides the contract.
3. **Hand-written client and server**: no tooling, guaranteed drift.

## Decision
Spec-first. `api/openapi/openapi.yaml` is edited before code; generated output lives in `gen/` folders and is never hand-edited (a hook enforces this).

**Generator (amendment, 2026-11-03):** `oapi-codegen` v2.8.2, run by `make gen` with `-generate types,std-http,strict-server`. Output: `services/api/internal/api/gen/api.gen.go`.
- `std-http` registers routes on `net/http`'s `ServeMux` using Go 1.22 `METHOD /path` patterns, so no router library is needed.
- `strict-server` gives one typed `…ResponseObject` per status code. A handler cannot return a status the spec does not declare.
- Spike result: the health route, served through the generated router and checked by curl, returns 200 at `/healthz` and 404 at `/api/v1/healthz`. The generated code is deterministic on regeneration.
- Rejected: `oapi-codegen` client-only output for now (the TypeScript clients come with the web slice, through a separate generator to be chosen then).

Path convention (amendment): infrastructure endpoints such as `/healthz` sit at the root. Product endpoints carry `/api/v1` in their path, and `servers` is `/`.

## Consequences
- Good: contract-first, typed server, docs for free, easy contract tests.
- Bad: generator quirks and a build step (`make gen`). The tool needs Go 1.26 to build, which the local toolchain fetches on demand.
- Follow-up: CI fails if generated code is stale (`make gen` then `git diff --exit-code`). TypeScript client generator is chosen with the web slice.
