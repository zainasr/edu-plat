-- name: GetSettings :one
SELECT register_due_time, edit_window_days, low_attendance_percent
FROM attendance_settings
WHERE tenant_id = sqlc.arg('tenant_id');

-- name: SetSettings :exec
-- The row is created the first time the settings are saved; until then reads use the defaults.
INSERT INTO attendance_settings (tenant_id, register_due_time, edit_window_days, low_attendance_percent, updated_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('register_due_time')::time, sqlc.arg('edit_window_days'),
        sqlc.arg('low_attendance_percent'), sqlc.arg('updated_by'))
ON CONFLICT (tenant_id) DO UPDATE
SET register_due_time = EXCLUDED.register_due_time, edit_window_days = EXCLUDED.edit_window_days,
    low_attendance_percent = EXCLUDED.low_attendance_percent, updated_at = now(), updated_by = EXCLUDED.updated_by;

-- name: InsertRegister :one
-- DO NOTHING on (section, day): a register that already exists returns no row.
INSERT INTO attendance_registers (tenant_id, section_id, day, created_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('section_id'), sqlc.arg('day'), sqlc.arg('created_by'))
ON CONFLICT (tenant_id, section_id, day) DO NOTHING
RETURNING id;

-- name: RegisterBySectionDay :one
SELECT id, section_id, day, status, submitted_at
FROM attendance_registers
WHERE tenant_id = sqlc.arg('tenant_id') AND section_id = sqlc.arg('section_id') AND day = sqlc.arg('day');

-- name: GetRegister :one
SELECT id, section_id, day, status, submitted_at
FROM attendance_registers
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id');

-- name: LockRegister :one
-- The row lock serialises marks, submit and corrections of one register.
SELECT id, section_id, day, status, submitted_at
FROM attendance_registers
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: SubmitRegister :execrows
UPDATE attendance_registers
SET status = 'submitted', submitted_at = now(), submitted_by = sqlc.arg('submitted_by')
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'open';

-- name: ListRegistersForDay :many
SELECT id, section_id, day, status, submitted_at
FROM attendance_registers
WHERE tenant_id = sqlc.arg('tenant_id') AND day = sqlc.arg('day')
ORDER BY section_id, id;

-- name: InsertRecord :exec
INSERT INTO attendance_records (tenant_id, register_id, student_id, version, status, method, reason, changed_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('register_id'), sqlc.arg('student_id'), sqlc.arg('version'),
        sqlc.arg('status'), sqlc.arg('method'), sqlc.narg('reason'), sqlc.arg('changed_by'));

-- name: CurrentRecords :many
-- The latest version of every student's record in the given registers, and whether it was ever corrected.
SELECT DISTINCT ON (r.register_id, r.student_id)
       r.register_id, r.student_id, r.status, r.method, r.version,
       EXISTS (SELECT 1 FROM attendance_records c
                WHERE c.tenant_id = r.tenant_id AND c.register_id = r.register_id AND c.student_id = r.student_id
                  AND c.method = 'correction') AS corrected
FROM attendance_records r
WHERE r.tenant_id = sqlc.arg('tenant_id') AND r.register_id = ANY(sqlc.arg('register_ids')::uuid[])
ORDER BY r.register_id, r.student_id, r.version DESC;

-- name: AbsentOnDay :many
-- Students whose current status in a SUBMITTED register of the day is absent.
SELECT cur.student_id, g.section_id
FROM (
    SELECT DISTINCT ON (r.register_id, r.student_id) r.register_id, r.student_id, r.status
    FROM attendance_records r
    JOIN attendance_registers g ON g.tenant_id = r.tenant_id AND g.id = r.register_id
    WHERE r.tenant_id = sqlc.arg('tenant_id') AND g.status = 'submitted' AND g.day = sqlc.arg('day')
    ORDER BY r.register_id, r.student_id, r.version DESC
) cur
JOIN attendance_registers g ON g.tenant_id = sqlc.arg('tenant_id') AND g.id = cur.register_id
WHERE cur.status = 'absent'
ORDER BY g.section_id, cur.student_id;

-- name: TotalsPerStudent :many
-- Present, late, absent and leave days per student over submitted registers in a range.
WITH cur AS (
    SELECT DISTINCT ON (r.register_id, r.student_id) r.student_id, r.status
    FROM attendance_records r
    JOIN attendance_registers g ON g.tenant_id = r.tenant_id AND g.id = r.register_id
    WHERE r.tenant_id = sqlc.arg('tenant_id') AND g.status = 'submitted'
      AND g.day BETWEEN sqlc.arg('from_day')::date AND sqlc.arg('to_day')::date
    ORDER BY r.register_id, r.student_id, r.version DESC
)
SELECT student_id,
       (count(*) FILTER (WHERE status = 'present'))::int AS present,
       (count(*) FILTER (WHERE status = 'late'))::int AS late,
       (count(*) FILTER (WHERE status = 'absent'))::int AS absent,
       (count(*) FILTER (WHERE status = 'leave'))::int AS leave
FROM cur
GROUP BY student_id;

-- name: StudentDays :many
-- One student's current status on each day of a range, from submitted registers.
SELECT g.day, g.section_id, cur.status, cur.method,
       EXISTS (SELECT 1 FROM attendance_records c
                WHERE c.tenant_id = cur.tenant_id AND c.register_id = cur.register_id AND c.student_id = cur.student_id
                  AND c.method = 'correction') AS corrected
FROM (
    SELECT DISTINCT ON (r.register_id) r.tenant_id, r.register_id, r.student_id, r.status, r.method
    FROM attendance_records r
    JOIN attendance_registers g ON g.tenant_id = r.tenant_id AND g.id = r.register_id
    WHERE r.tenant_id = sqlc.arg('tenant_id') AND r.student_id = sqlc.arg('student_id') AND g.status = 'submitted'
      AND g.day BETWEEN sqlc.arg('from_day')::date AND sqlc.arg('to_day')::date
    ORDER BY r.register_id, r.version DESC
) cur
JOIN attendance_registers g ON g.tenant_id = cur.tenant_id AND g.id = cur.register_id
WHERE cur.status <> 'left'
ORDER BY g.day, g.section_id;

-- name: CountedElsewhere :many
-- Students already counted in ANOTHER section's submitted register on the same day (a same-day move),
-- so they are not counted twice. A student who left that register is not counted there.
SELECT DISTINCT cur.student_id
FROM (
    SELECT DISTINCT ON (r.register_id, r.student_id) r.student_id, r.status
    FROM attendance_records r
    JOIN attendance_registers g ON g.tenant_id = r.tenant_id AND g.id = r.register_id
    WHERE r.tenant_id = sqlc.arg('tenant_id') AND g.status = 'submitted' AND g.day = sqlc.arg('day')
      AND g.section_id <> sqlc.arg('section_id') AND r.student_id = ANY(sqlc.arg('student_ids')::uuid[])
    ORDER BY r.register_id, r.student_id, r.version DESC
) cur
WHERE cur.status <> 'left';
