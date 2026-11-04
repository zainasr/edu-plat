# ADR-0011: Walking-skeleton auth, tenant transactions and data access

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-03 |
| Spec links | DEC-002, DEC-004, DEC-006, DEC-014, REQ-IAM-010, 015, REQ-API-011, 017, US-IAM-003, US-IAM-004, ADR-0002 |

## Context
The first vertical slice (tenant → OTP sign-in → token → create student with guardian → read with curl) forces decisions about login, token format, how RLS is enforced, and how Go talks to Postgres. Each choice is small but hard to reverse once clients depend on it.

## Decisions

**1. Two runtime roles.** The migration owner (`app`) creates and owns tables. The API connects as `app_rt`, which is `NOSUPERUSER NOBYPASSRLS` and is not the table owner, so every RLS policy applies to it. The `app` role is used only by migrations and `cmd/bootstrap`. Table owners bypass RLS unless `FORCE ROW LEVEL SECURITY` is set, so the owner role must not be the API role.

**2. Tenant context per transaction.** Every tenant query runs in `platform/db.WithTenant`: it begins a transaction, runs `set_config('app.tenant_id', $1, true)` (transaction-local), and runs the work. Policies read `current_setting('app.tenant_id', true)`. With no tenant set, the value is NULL and no rows match, so the default is to fail closed.

**3. Login before tenant is known.** Looking up a token, or a tenant by slug, must happen before a tenant is known. Two `SECURITY DEFINER` functions cover this, and they are the only controlled cross-tenant read paths: `resolve_api_token(hash)` and `tenant_id_by_slug(slug)`. They return the minimum columns and are granted `EXECUTE` to `app_rt` only.

**4. OTP login.** `POST /api/v1/auth/otp/request` creates a 6-digit challenge for a normalised E.164 mobile. The code is stored as HMAC-SHA256 with a server key (`AUTH_SECRET`), never in clear. The code expires after 5 minutes (US-IAM-003) and locks after 5 wrong attempts. Delivery goes through an `OTPSender` interface. The only sender in this slice logs the code and only runs when `APP_ENV=dev`. The process refuses to start in any other environment until a real sender exists. Logs never contain the code or the full mobile (masked).

**5. Tokens.** One table, `api_tokens`, holds two kinds. `kind=session` is issued by OTP verify with a 12-hour expiry (web idle rule; mobile refresh comes later). `kind=pat` is a personal access token created by a signed-in user (REQ-IAM-015). Token format: `edu_<kind>_<43 base64url chars>`, 256 random bits. Only the SHA-256 of the full token is stored, since the token is high-entropy and a fast hash is safe here. Tokens are revocable (`revoked_at`) and record `last_used_at`. The prefix lets secret scanners and humans tell tokens apart.

**6. Permissions.** Roles are code constants in this slice (`owner` only, with a named permission list). Assignments are data (`role_assignments`, tenant-scoped). The effective permission set is the union over a membership's assignments (REQ-IAM-002). Handlers check named permissions, never role names.

**7. Data access.** `sqlc` generates typed Go from SQL files, using the `pgx/v5` driver. Handlers call services, services call stores, stores run sqlc queries inside the tenant transaction. Migrations are goose SQL (ADR-0009), and sqlc reads them as the schema.

**8. Dependencies.** Two new runtime dependencies, each justified:
- `github.com/jackc/pgx/v5` v5.12.0: the Postgres driver and pool. The standard library has no Postgres driver.
- `github.com/oapi-codegen/runtime` v1.8.0 (with `github.com/google/uuid` it pulls in): the types the generated server uses for UUID and date parameters. The generator emits imports of it, so it is required to build `internal/api/gen`.

Everything else is the standard library. The module moves to `go 1.26`, because pgx v5.12 requires Go 1.25 and the pinned tools require 1.26. CI follows `go.mod`, so it picks this up.

## Options considered
1. **Shared `app` role for everything**: simplest, but RLS is silently bypassed. Rejected.
2. **Set tenant with a pool-level `SET`**: leaks between pooled connections. Transaction-local `set_config` avoids that.
3. **Opaque tokens looked up by an unscoped query**: needs an RLS bypass for every lookup. A single definer function keeps the bypass in one reviewed place.
4. **JWT access tokens**: cannot be revoked immediately (US-IAM-005 needs instant revocation). Rejected.
5. **ORM (GORM or similar)**: hides the tenant filter and the SQL that money code needs to read (ADR-0004). Rejected.

## Consequences
- Good: RLS cannot be bypassed by a missing filter, because the API role has no bypass. Tokens revoke instantly. Every query is plain, reviewable SQL.
- Bad: `SECURITY DEFINER` functions are powerful and need review each time they change. Migrations need a second role, so local setup needs the `deploy/initdb` role script.
- Follow-up: the integration test `TestRLS_TenantBCannotSeeTenantA` proves isolation with the real `app_rt` role. Tenant and user admin paths come later. Revisit sessions (refresh, device limits) with the mobile slice.
