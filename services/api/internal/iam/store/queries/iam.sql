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
WHERE tenant_id = sqlc.arg('tenant_id') AND membership_id = sqlc.arg('membership_id')
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
