-- name: FindUserByMobile :one
SELECT id, mobile, full_name
FROM users
WHERE mobile = $1;

-- name: InsertOTPChallenge :one
INSERT INTO otp_challenges (mobile, code_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id;

-- name: LatestOTPChallenge :one
SELECT id, code_hash, expires_at, attempts, consumed_at
FROM otp_challenges
WHERE mobile = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementOTPAttempts :exec
UPDATE otp_challenges
SET attempts = attempts + 1
WHERE id = $1;

-- name: ConsumeOTPChallenge :execrows
-- Conditional update: of two concurrent verifies with the same code, only one gets 1 row back.
UPDATE otp_challenges
SET consumed_at = now()
WHERE id = $1 AND consumed_at IS NULL;

-- name: ActiveMembership :one
SELECT id
FROM memberships
WHERE tenant_id = sqlc.arg('tenant_id') AND user_id = sqlc.arg('user_id') AND status = 'active';

-- name: RoleNamesForMembership :many
SELECT role
FROM role_assignments
WHERE tenant_id = sqlc.arg('tenant_id') AND membership_id = sqlc.arg('membership_id') AND revoked_at IS NULL
ORDER BY role;

-- name: InsertAPIToken :one
INSERT INTO api_tokens (tenant_id, membership_id, user_id, kind, name, token_hash, expires_at)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('membership_id'), sqlc.arg('user_id'),
        sqlc.arg('kind'), sqlc.arg('name'), sqlc.arg('token_hash'), sqlc.arg('expires_at'))
RETURNING id, expires_at;

-- name: TouchAPIToken :exec
UPDATE api_tokens
SET last_used_at = now()
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id');

-- name: InsertTenant :one
INSERT INTO tenants (slug, name)
VALUES ($1, $2)
RETURNING id;

-- name: InsertUser :one
INSERT INTO users (mobile, full_name)
VALUES ($1, $2)
RETURNING id;

-- name: InsertMembership :one
INSERT INTO memberships (tenant_id, user_id)
VALUES ($1, $2)
RETURNING id;

-- name: InsertRoleAssignment :exec
INSERT INTO role_assignments (tenant_id, membership_id, role)
VALUES ($1, $2, $3);

-- ---- staff management (slice 4b) ------------------------------------------------------

-- name: EnsureUser :one
-- The narrow definer function: adds the person if the mobile is new, else returns the existing id.
SELECT ensure_user(sqlc.arg('mobile'), sqlc.arg('full_name'))::uuid AS id;

-- name: InsertStaffMembership :one
INSERT INTO memberships (tenant_id, user_id, display_name, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('user_id'), sqlc.arg('display_name'), sqlc.arg('created_by'))
ON CONFLICT (tenant_id, user_id) DO NOTHING
RETURNING id;

-- name: AssignRole :one
-- Fails with a unique violation when the person already holds the role (live assignments only).
INSERT INTO role_assignments (tenant_id, membership_id, role, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('membership_id'), sqlc.arg('role'), sqlc.arg('created_by'))
RETURNING id;

-- name: RevokeRole :one
UPDATE role_assignments
SET revoked_at = now(), revoked_by = sqlc.arg('revoked_by')
WHERE tenant_id = sqlc.arg('tenant_id') AND membership_id = sqlc.arg('membership_id')
  AND role = sqlc.arg('role') AND revoked_at IS NULL
RETURNING id;

-- name: ListStaff :many
SELECT m.id, m.user_id, u.mobile, COALESCE(m.display_name, u.full_name)::text AS full_name,
       m.status, m.created_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('status')::text IS NULL OR m.status = sqlc.narg('status')::text)
ORDER BY COALESCE(m.display_name, u.full_name), m.id;

-- name: GetStaff :one
SELECT m.id, m.user_id, u.mobile, COALESCE(m.display_name, u.full_name)::text AS full_name,
       m.status, m.created_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.tenant_id = sqlc.arg('tenant_id') AND m.id = sqlc.arg('id');

-- name: LiveRolesForMemberships :many
SELECT membership_id, role
FROM role_assignments
WHERE tenant_id = sqlc.arg('tenant_id') AND membership_id = ANY(sqlc.arg('membership_ids')::uuid[]) AND revoked_at IS NULL
ORDER BY membership_id, role;

-- name: LockOwnerChanges :exec
-- One lock per school for anything that can remove an owner, so two requests cannot each see "there is
-- another owner" and both remove one. Released at commit.
SELECT pg_advisory_xact_lock(hashtextextended('owners:' || sqlc.arg('tenant_id')::text, 0));

-- name: CountActiveOwners :one
SELECT count(*)::bigint
FROM role_assignments ra
JOIN memberships m ON m.tenant_id = ra.tenant_id AND m.id = ra.membership_id
WHERE ra.tenant_id = sqlc.arg('tenant_id') AND ra.role = 'owner' AND ra.revoked_at IS NULL AND m.status = 'active';

-- name: DeactivateMembership :one
UPDATE memberships
SET status = 'deactivated', deactivated_at = now(), deactivated_by = sqlc.arg('deactivated_by')
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'active'
RETURNING id;

-- name: ReactivateMembership :one
UPDATE memberships
SET status = 'active', deactivated_at = NULL, deactivated_by = NULL
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'deactivated'
RETURNING id;

-- name: RevokeMembershipTokens :execrows
UPDATE api_tokens
SET revoked_at = now()
WHERE tenant_id = sqlc.arg('tenant_id') AND membership_id = sqlc.arg('membership_id') AND revoked_at IS NULL;
