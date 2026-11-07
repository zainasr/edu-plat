-- name: NextCounter :one
-- Atomic per-tenant counter: one statement, safe under concurrent enrolment.
INSERT INTO tenant_counters (tenant_id, name, value)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('name'), 1)
ON CONFLICT (tenant_id, name) DO UPDATE SET value = tenant_counters.value + 1
RETURNING value;

-- name: InsertFamily :one
INSERT INTO families (tenant_id, number, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('number'), sqlc.arg('created_by'))
RETURNING id;

-- name: GuardianByMobile :one
SELECT id, family_id, full_name, mobile
FROM guardians
WHERE tenant_id = sqlc.arg('tenant_id') AND mobile = sqlc.arg('mobile');

-- name: InsertGuardian :one
INSERT INTO guardians (tenant_id, family_id, full_name, mobile, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('family_id'), sqlc.arg('full_name'),
        sqlc.arg('mobile'), sqlc.arg('created_by'))
RETURNING id;

-- name: InsertStudent :one
INSERT INTO students (tenant_id, family_id, registration_no, full_name, father_name,
                      gender, date_of_birth, created_by, updated_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('family_id'), sqlc.arg('registration_no'),
        sqlc.arg('full_name'), sqlc.arg('father_name'), sqlc.arg('gender'),
        sqlc.arg('date_of_birth'), sqlc.arg('created_by'), sqlc.arg('created_by'))
RETURNING id, created_at;

-- name: InsertGuardianLink :exec
INSERT INTO guardian_links (tenant_id, student_id, guardian_id, relationship, is_primary,
                            is_fee_payer, receives_notifications, emergency_contact, access, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('student_id'), sqlc.arg('guardian_id'),
        sqlc.arg('relationship'), sqlc.arg('is_primary'), sqlc.arg('is_fee_payer'),
        sqlc.arg('receives_notifications'), sqlc.arg('emergency_contact'), sqlc.arg('access'),
        sqlc.arg('created_by'));

-- name: GetStudent :one
SELECT s.id, s.registration_no, s.full_name, s.father_name, s.gender,
       s.date_of_birth, s.status, s.family_id, s.created_at, f.number AS family_number
FROM students s
JOIN families f ON f.tenant_id = s.tenant_id AND f.id = s.family_id
WHERE s.tenant_id = sqlc.arg('tenant_id') AND s.id = sqlc.arg('id');

-- name: ListStudentsPage :many
-- Keyset pagination on (created_at, id), newest first. A NULL cursor means the first page.
SELECT s.id, s.registration_no, s.full_name, s.father_name, s.gender,
       s.date_of_birth, s.status, s.family_id, s.created_at, f.number AS family_number
FROM students s
JOIN families f ON f.tenant_id = s.tenant_id AND f.id = s.family_id
WHERE s.tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('after_created_at')::timestamptz IS NULL
       OR (s.created_at, s.id) < (sqlc.narg('after_created_at')::timestamptz, sqlc.narg('after_id')::uuid))
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg('page_limit');

-- name: ListGuardianLinksForStudents :many
SELECT l.student_id, l.guardian_id, g.full_name, g.mobile, l.relationship,
       l.is_primary, l.is_fee_payer, l.receives_notifications, l.emergency_contact, l.access
FROM guardian_links l
JOIN guardians g ON g.tenant_id = l.tenant_id AND g.id = l.guardian_id
WHERE l.tenant_id = sqlc.arg('tenant_id') AND l.student_id = ANY(sqlc.arg('student_ids')::uuid[])
ORDER BY l.is_primary DESC, g.full_name;

-- name: InsertEnrolment :one
INSERT INTO enrolments (tenant_id, student_id, session_id, section_id, started_on, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('student_id'), sqlc.arg('session_id'), sqlc.arg('section_id'),
        sqlc.arg('started_on'), sqlc.arg('created_by'))
RETURNING id;

-- name: CloseEnrolment :execrows
-- Conditional: only an active enrolment can be closed, so a double move cannot close it twice.
UPDATE enrolments
SET status = 'closed', ended_on = sqlc.arg('ended_on'), reason = sqlc.arg('reason')
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'active';

-- name: ActiveEnrolmentForStudent :one
SELECT id, session_id, section_id
FROM enrolments
WHERE tenant_id = sqlc.arg('tenant_id') AND student_id = sqlc.arg('student_id') AND status = 'active'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListEnrolmentsForStudents :many
-- Newest first. For a page of students, the first active row per student is the current enrolment.
SELECT id, student_id, session_id, section_id, status, started_on, ended_on, reason
FROM enrolments
WHERE tenant_id = sqlc.arg('tenant_id') AND student_id = ANY(sqlc.arg('student_ids')::uuid[])
ORDER BY created_at DESC, id DESC;
