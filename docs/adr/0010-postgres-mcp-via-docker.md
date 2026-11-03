# ADR-0010: Postgres MCP server runs from a pinned Docker image

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-03 |
| Spec links | ADR-0008 (agent workflow), ADR-0002 (tenant isolation) |

## Context
`.mcp.json` gives Claude read access to the local database through the Postgres MCP server, in `restricted` mode. The first config ran `uvx postgres-mcp`, which failed on startup. Two causes were found:
- The PyPI package named `postgres-mcp` is a different project ("PostgreSQL Tuning and Analysis Tool"). It takes a bare DSN argument and has no `--access-mode`.
- The upstream project (crystaldba/postgres-mcp) installs from Git, but its tags pin `pglast` versions (7.2.0, 7.11) that are not published, so `uvx --from git+…` cannot resolve them.

## Options considered
1. **Docker image `crystaldba/postgres-mcp`, pinned by digest**: the upstream project's own supported path. Needs only Docker, which the repo already uses. Host networking reaches the compose database.
2. **`uvx --from git+…@tag`**: fails today because of the unpublished `pglast` pins. It would need an override file that we would maintain.
3. **A hand-written MCP server**: full control, but more code to own, for a problem already solved upstream.

## Decision
Run the server from `crystaldba/postgres-mcp` pinned by digest, with `--network host` and `--access-mode=restricted`, reading `DATABASE_URI` from the environment. The digest is recorded in `.mcp.json`. Playwright MCP is pinned to `@playwright/mcp@0.0.84` for the same reason: `@latest` plus a corrupted npx cache failed to start.

## Consequences
- Good: works on this machine today; the server cannot write (restricted mode); the version is fixed.
- Bad: `--network host` works on Linux only. On macOS the container needs `host.docker.internal` in the DSN instead. Updating the image means updating the digest in the same PR.
- Follow-up: when upstream publishes a release that resolves on PyPI or Git, switch back to `uvx` and record that here.
