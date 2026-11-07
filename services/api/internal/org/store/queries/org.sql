-- name: EnsureDefaultBranch :exec
-- Creates the tenant's default branch the first time it is needed. Safe to run twice:
-- the partial unique index branches_one_default turns the second insert into a no-op.
INSERT INTO branches (tenant_id, name, is_default)
VALUES (sqlc.arg('tenant_id'), 'Main', true)
ON CONFLICT (tenant_id) WHERE is_default DO NOTHING;

-- name: DefaultBranch :one
SELECT id
FROM branches
WHERE tenant_id = sqlc.arg('tenant_id') AND is_default;

-- name: InsertSession :one
INSERT INTO sessions (tenant_id, branch_id, name, starts_on, ends_on, is_current, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('branch_id'), sqlc.arg('name'), sqlc.arg('starts_on'),
        sqlc.arg('ends_on'), sqlc.arg('is_current'), sqlc.arg('created_by'))
RETURNING id, name, starts_on, ends_on, is_current;

-- name: CurrentSession :one
SELECT id, name, starts_on, ends_on, is_current
FROM sessions
WHERE tenant_id = sqlc.arg('tenant_id') AND branch_id = sqlc.arg('branch_id') AND is_current;

-- name: ListSessions :many
SELECT id, name, starts_on, ends_on, is_current
FROM sessions
WHERE tenant_id = sqlc.arg('tenant_id') AND branch_id = sqlc.arg('branch_id')
ORDER BY starts_on DESC, id DESC;

-- name: ClearCurrentSession :exec
UPDATE sessions
SET is_current = false
WHERE tenant_id = sqlc.arg('tenant_id') AND branch_id = sqlc.arg('branch_id') AND is_current;

-- name: SetCurrentSession :one
UPDATE sessions
SET is_current = true
WHERE tenant_id = sqlc.arg('tenant_id') AND branch_id = sqlc.arg('branch_id') AND id = sqlc.arg('id')
RETURNING id, name, starts_on, ends_on, is_current;

-- name: SessionsByIDs :many
SELECT id, name, starts_on, ends_on, is_current
FROM sessions
WHERE tenant_id = sqlc.arg('tenant_id') AND id = ANY(sqlc.arg('ids')::uuid[]);

-- name: InsertClass :one
INSERT INTO classes (tenant_id, branch_id, name, grade_level, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('branch_id'), sqlc.arg('name'), sqlc.arg('grade_level'),
        sqlc.arg('created_by'))
RETURNING id, name, grade_level, status;

-- name: ListClasses :many
SELECT id, name, grade_level, status
FROM classes
WHERE tenant_id = sqlc.arg('tenant_id') AND branch_id = sqlc.arg('branch_id')
ORDER BY grade_level, name;

-- name: GetClass :one
SELECT id, name, grade_level, status
FROM classes
WHERE tenant_id = sqlc.arg('tenant_id') AND branch_id = sqlc.arg('branch_id') AND id = sqlc.arg('id');

-- name: ArchiveClass :one
UPDATE classes
SET status = 'archived'
WHERE tenant_id = sqlc.arg('tenant_id') AND branch_id = sqlc.arg('branch_id') AND id = sqlc.arg('id')
RETURNING id, name, grade_level, status;

-- name: InsertSection :one
INSERT INTO sections (tenant_id, class_id, name, shift, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('class_id'), sqlc.arg('name'), sqlc.arg('shift'),
        sqlc.arg('created_by'))
RETURNING id;

-- name: ArchiveSection :one
UPDATE sections
SET status = 'archived'
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
RETURNING id;

-- name: ListSections :many
SELECT s.id, s.class_id, c.name AS class_name, s.name, s.shift, s.status
FROM sections s
JOIN classes c ON c.tenant_id = s.tenant_id AND c.id = s.class_id
WHERE s.tenant_id = sqlc.arg('tenant_id') AND c.branch_id = sqlc.arg('branch_id')
  AND (sqlc.narg('class_id')::uuid IS NULL OR s.class_id = sqlc.narg('class_id')::uuid)
ORDER BY c.grade_level, c.name, s.name, s.shift;

-- name: SectionsByIDs :many
-- Used by other modules, through the org service, to describe the sections they reference.
SELECT s.id, s.class_id, c.name AS class_name, c.status AS class_status, c.branch_id,
       s.name, s.shift, s.status
FROM sections s
JOIN classes c ON c.tenant_id = s.tenant_id AND c.id = s.class_id
WHERE s.tenant_id = sqlc.arg('tenant_id') AND s.id = ANY(sqlc.arg('ids')::uuid[]);
