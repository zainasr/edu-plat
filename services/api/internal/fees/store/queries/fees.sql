-- name: EnsureDefaultFeeTypes :exec
-- The suggested set, created the first time a school looks at its fee types. They are suggestions,
-- never locks: a school archives what it does not use. ON CONFLICT DO NOTHING means an archived
-- suggestion stays archived, and a type the school created first is not overwritten.
INSERT INTO fee_types (tenant_id, code, name, kind, is_deposit)
SELECT sqlc.arg('tenant_id')::uuid, v.code, v.name, v.kind, v.is_deposit
FROM (VALUES
    ('tuition',          'Tuition',          'tuition',          false),
    ('admission',        'Admission Fee',    'admission',        false),
    ('registration',     'Registration Fee', 'registration',     false),
    ('annual',           'Annual Fee',       'annual',           false),
    ('transport',        'Transport',        'transport',        false),
    ('security_deposit', 'Security Deposit', 'security_deposit', true)
) AS v(code, name, kind, is_deposit)
ON CONFLICT (tenant_id, code) DO NOTHING;

-- name: ListFeeTypes :many
SELECT id, code, name, kind, is_deposit, status
FROM fee_types
WHERE tenant_id = sqlc.arg('tenant_id')
ORDER BY name, id;

-- name: GetFeeType :one
SELECT id, code, name, kind, is_deposit, status
FROM fee_types
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id');

-- name: InsertFeeType :one
INSERT INTO fee_types (tenant_id, code, name, kind, is_deposit, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('kind'),
        sqlc.arg('is_deposit'), sqlc.arg('created_by'))
RETURNING id, code, name, kind, is_deposit, status;

-- name: ArchiveFeeType :one
UPDATE fee_types
SET status = 'archived'
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
RETURNING id, code, name, kind, is_deposit, status;

-- name: InsertPlanLine :one
INSERT INTO fee_plan_lines (tenant_id, class_id, fee_type_id, amount, frequency, effective_from, removed, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('class_id'), sqlc.arg('fee_type_id'), sqlc.arg('amount'),
        sqlc.arg('frequency'), sqlc.arg('effective_from'), sqlc.arg('removed'), sqlc.arg('created_by'))
RETURNING id;

-- name: ListPlanVersions :many
-- Every version ever set on a class plan, newest first, with the fee type's name and kind.
SELECT p.fee_type_id, t.name AS fee_type_name, t.kind, p.amount, p.frequency, p.effective_from, p.removed
FROM fee_plan_lines p
JOIN fee_types t ON t.tenant_id = p.tenant_id AND t.id = p.fee_type_id
WHERE p.tenant_id = sqlc.arg('tenant_id') AND p.class_id = sqlc.arg('class_id')
ORDER BY p.effective_from DESC, p.created_at DESC, p.id DESC;

-- name: InsertOverride :one
INSERT INTO student_fee_overrides (tenant_id, student_id, fee_type_id, amount, effective_from, cleared, reason, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('student_id'), sqlc.arg('fee_type_id'), sqlc.arg('amount'),
        sqlc.arg('effective_from'), sqlc.arg('cleared'), sqlc.arg('reason'), sqlc.arg('created_by'))
RETURNING id, created_at;

-- name: ListOverridesForStudent :many
SELECT o.fee_type_id, t.name AS fee_type_name, o.amount, o.effective_from, o.cleared, o.reason, o.created_at
FROM student_fee_overrides o
JOIN fee_types t ON t.tenant_id = o.tenant_id AND t.id = o.fee_type_id
WHERE o.tenant_id = sqlc.arg('tenant_id') AND o.student_id = sqlc.arg('student_id')
ORDER BY o.effective_from DESC, o.created_at DESC, o.id DESC;
