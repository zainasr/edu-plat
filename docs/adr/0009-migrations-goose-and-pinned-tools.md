# ADR-0009: Migrations with goose and pinned developer tools

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-03 |
| Spec links | DEC-005, TECH (migrations), ADR-0002, ADR-0008 |

## Context
Tenant tables need forward-only, backward-compatible SQL migrations, and every tenant table needs an RLS policy. Migration files must be plain SQL that reviewers can read line by line. Developer tools (lint, sqlc, goose, oapi-codegen) must be the same version on every machine and in CI, or generated code and lint results drift.

## Options considered
1. **goose**: plain `.sql` files with `-- +goose Up` and `-- +goose Down` markers, a small CLI, runs in CI and locally. Simple and close to the database.
2. **Atlas**: declarative schema diff with a richer tool. More to learn, and it encourages editing a desired-state file rather than writing forward steps.
3. **golang-migrate**: plain SQL, but no built-in Go migrations and fewer conveniences for the Up/Down convention.

## Decision
Use **goose** (v3.29.0). Migrations live in `services/api/migrations/` as numbered `.sql` files. An applied migration is never edited; the `guard-paths` hook enforces this. Developer tools are pinned in the `Makefile` (`GOLANGCI_LINT_VERSION`, `SQLC_VERSION`, `GOOSE_VERSION`, `OAPI_CODEGEN_VERSION`) and installed with `make tools`.

Pinned versions (2026-11-03): golangci-lint v2.15.0, sqlc v1.31.1, goose v3.29.0, oapi-codegen v2.8.2. These four need Go 1.26 to build, so the local toolchain fetches go1.26.10 on demand. The project's own `go` directive stays at 1.23 until a decision to raise it.

## Consequences
- Good: readable migrations, one version per tool, reproducible CI.
- Bad: each tool version bump is a PR; Down migrations are only used in dev and are not relied on in production (forward-only policy).
- Follow-up: revisit goose if a migration needs something it cannot express. Decide whether to raise the project's Go version to 1.26 when the next Go feature is needed (open item for the owner).
