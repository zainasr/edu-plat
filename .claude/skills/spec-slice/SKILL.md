---
name: spec-slice
description: Implement one vertical slice of the product from the locked spec (API + logic + SQL + tests). Use when asked to build a spec module, requirement or user story.
argument-hint: "[module or story ID, e.g. US-FEE-002]"
disable-model-invocation: true
---
Build the slice: $ARGUMENTS

1. Read the module file in `docs/spec/03-modules/` and note the REQ, BR and US IDs and Given/When/Then criteria. Read `docs/spec/02-cross-cutting/04-api-conventions.md`.
2. Update `api/openapi/openapi.yaml` first (paths, schemas, error codes, permission, idempotency).
3. Write tests from the acceptance criteria (table-driven, named with the spec ID). Run them and see them fail.
4. Implement migration (with tenant_id + RLS), sqlc queries, service, handler. Keep within `.claude/rules/go.md`.
5. Run `make check`. Show the result.
6. Add a learning note with `/learning-note` and an ADR with `/adr` if a new technical choice was made.
7. Report which REQ/BR/US IDs are now covered and which are not.
