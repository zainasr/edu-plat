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

-- name: NextChallanNo :one
-- Gapless: the counter moves inside the release transaction, so a rolled-back release gives its
-- number back.
INSERT INTO tenant_counters (tenant_id, name, value)
VALUES (sqlc.arg('tenant_id'), 'challan', 1)
ON CONFLICT (tenant_id, name) DO UPDATE SET value = tenant_counters.value + 1
RETURNING value;

-- name: LiveChallanStudents :many
-- Students who already have a draft or released challan for the month.
SELECT student_id
FROM challans
WHERE tenant_id = sqlc.arg('tenant_id') AND month = sqlc.arg('month') AND status <> 'void';

-- name: InsertChallan :one
-- DO NOTHING on the one-live-challan-per-month index: a student who already has one returns no row.
INSERT INTO challans (tenant_id, student_id, family_id, month, due_date, total, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('student_id'), sqlc.arg('family_id'), sqlc.arg('month'),
        sqlc.arg('due_date'), sqlc.arg('total'), sqlc.arg('created_by'))
ON CONFLICT (tenant_id, student_id, month) WHERE status <> 'void' DO NOTHING
RETURNING id;

-- name: InsertChallanLine :exec
INSERT INTO challan_lines (tenant_id, challan_id, line_no, fee_type_id, fee_type_name, kind, frequency, amount)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('challan_id'), sqlc.arg('line_no'), sqlc.arg('fee_type_id'),
        sqlc.arg('fee_type_name'), sqlc.arg('kind'), sqlc.arg('frequency'), sqlc.arg('amount'));

-- name: GetChallan :one
SELECT id, student_id, family_id, month, due_date, status, total, challan_no, consumer_no,
       released_at, void_reason, created_at
FROM challans
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id');

-- name: LockChallan :one
-- Takes the row lock, so two releases of the same challan run one after the other.
SELECT id, status
FROM challans
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: ListChallansPage :many
-- Keyset pagination on (created_at, id), newest first. A NULL cursor or filter means "no condition".
SELECT id, student_id, family_id, month, due_date, status, total, challan_no, consumer_no,
       released_at, void_reason, created_at
FROM challans
WHERE tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('month')::date IS NULL OR month = sqlc.narg('month')::date)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('student_id')::uuid IS NULL OR student_id = sqlc.narg('student_id')::uuid)
  AND (sqlc.narg('after_created_at')::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg('after_created_at')::timestamptz, sqlc.narg('after_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit');

-- name: ListChallanLines :many
SELECT challan_id, line_no, fee_type_id, fee_type_name, kind, frequency, amount
FROM challan_lines
WHERE tenant_id = sqlc.arg('tenant_id') AND challan_id = ANY(sqlc.arg('challan_ids')::uuid[])
ORDER BY challan_id, line_no;

-- name: ReleaseChallan :one
-- Conditional on draft: with the row lock taken first, this cannot run twice for one challan.
UPDATE challans
SET status = 'released', challan_no = sqlc.arg('challan_no'), consumer_no = sqlc.arg('consumer_no'),
    released_at = now(), released_by = sqlc.arg('released_by')
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'draft'
RETURNING id;

-- name: VoidChallan :one
UPDATE challans
SET status = 'void', void_reason = sqlc.arg('reason'), voided_at = now()
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'draft'
RETURNING id;

-- name: FirstVersionMonths :many
-- When each fee type first took effect on a class plan: it anchors annual fees to a month of the year.
SELECT fee_type_id, min(effective_from)::date AS first_from
FROM fee_plan_lines
WHERE tenant_id = sqlc.arg('tenant_id') AND class_id = sqlc.arg('class_id')
GROUP BY fee_type_id;
