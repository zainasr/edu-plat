-- name: GetSettings :one
SELECT register_due_time, edit_window_days, low_attendance_percent, morning_start, evening_start, late_after_minutes
FROM attendance_settings
WHERE tenant_id = sqlc.arg('tenant_id');

-- name: SetSettings :exec
-- The row is created the first time the settings are saved; until then reads use the defaults.
INSERT INTO attendance_settings (tenant_id, register_due_time, edit_window_days, low_attendance_percent,
                                 morning_start, evening_start, late_after_minutes, updated_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('register_due_time')::time, sqlc.arg('edit_window_days'),
        sqlc.arg('low_attendance_percent'), sqlc.arg('morning_start')::time, sqlc.arg('evening_start')::time,
        sqlc.arg('late_after_minutes'), sqlc.arg('updated_by'))
ON CONFLICT (tenant_id) DO UPDATE
SET register_due_time = EXCLUDED.register_due_time, edit_window_days = EXCLUDED.edit_window_days,
    low_attendance_percent = EXCLUDED.low_attendance_percent, morning_start = EXCLUDED.morning_start,
    evening_start = EXCLUDED.evening_start, late_after_minutes = EXCLUDED.late_after_minutes,
    updated_at = now(), updated_by = EXCLUDED.updated_by;

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

-- ---- cards and the gate scan (slice 6b) -----------------------------------------------

-- name: DisableActiveCards :execrows
-- Issuing a new card disables the student's current one first (one active card per student).
UPDATE student_cards
SET status = 'disabled', disabled_at = now(), disabled_by = sqlc.arg('disabled_by'), disabled_reason = sqlc.arg('reason')
WHERE tenant_id = sqlc.arg('tenant_id') AND student_id = sqlc.arg('student_id') AND status = 'active';

-- name: InsertCard :one
INSERT INTO student_cards (tenant_id, student_id, issued_by)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('student_id'), sqlc.arg('issued_by'))
RETURNING id, issued_at;

-- name: GetCard :one
SELECT id, student_id, status, issued_at, disabled_at, disabled_reason
FROM student_cards
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id');

-- name: ShareCard :one
-- FOR SHARE: a scan holds the card until it commits, so disabling it (FOR UPDATE) waits for scans in flight
-- and every scan that commits before the disable returns was made while the card was still active.
SELECT id, student_id, status, issued_at, disabled_at, disabled_reason
FROM student_cards
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
FOR SHARE;

-- name: LockCard :one
SELECT id, student_id, status
FROM student_cards
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id')
FOR UPDATE;

-- name: DisableCard :execrows
UPDATE student_cards
SET status = 'disabled', disabled_at = now(), disabled_by = sqlc.arg('disabled_by'), disabled_reason = sqlc.arg('reason')
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND status = 'active';

-- name: ListCardsForStudent :many
SELECT id, student_id, status, issued_at, disabled_at, disabled_reason
FROM student_cards
WHERE tenant_id = sqlc.arg('tenant_id') AND student_id = sqlc.arg('student_id')
ORDER BY issued_at DESC, id DESC;

-- name: LockScanKey :exec
-- One lock per student and day, so a double tap at the gate cannot record two arrivals.
SELECT pg_advisory_xact_lock(hashtextextended('gate-scan:' || sqlc.arg('student_id')::text || ':' || sqlc.arg('day')::text, 0));

-- name: ScanByClientID :one
SELECT id, student_id, card_id, section_id, scanned_at, status
FROM gate_scans
WHERE tenant_id = sqlc.arg('tenant_id') AND client_scan_id = sqlc.arg('client_scan_id');

-- name: ScanTimesForStudentDay :many
SELECT scanned_at
FROM gate_scans
WHERE tenant_id = sqlc.arg('tenant_id') AND student_id = sqlc.arg('student_id') AND day = sqlc.arg('day')
ORDER BY scanned_at;

-- name: InsertScan :one
INSERT INTO gate_scans (tenant_id, student_id, card_id, section_id, day, scanned_at, status, scanned_by, client_scan_id)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('student_id'), sqlc.arg('card_id'), sqlc.arg('section_id'), sqlc.arg('day'),
        sqlc.arg('scanned_at'), sqlc.arg('status'), sqlc.arg('scanned_by'), sqlc.narg('client_scan_id'))
RETURNING id;

-- name: FirstScanOfDay :one
-- A student's arrival: their earliest scan of the day.
SELECT scanned_at, status
FROM gate_scans
WHERE tenant_id = sqlc.arg('tenant_id') AND student_id = sqlc.arg('student_id') AND day = sqlc.arg('day')
ORDER BY scanned_at, id
LIMIT 1;

-- name: ListScansForDay :many
SELECT id, student_id, section_id, scanned_at, status
FROM gate_scans
WHERE tenant_id = sqlc.arg('tenant_id') AND day = sqlc.arg('day')
ORDER BY scanned_at, id;

-- name: ArrivalsOnDay :many
-- Each given student's first scan of the day.
SELECT DISTINCT ON (student_id) student_id, scanned_at, status
FROM gate_scans
WHERE tenant_id = sqlc.arg('tenant_id') AND day = sqlc.arg('day') AND student_id = ANY(sqlc.arg('student_ids')::uuid[])
ORDER BY student_id, scanned_at, id;

-- name: MismatchesOnDay :many
-- Gate against class, from submitted registers only. A student scanned but marked absent; or marked
-- present or late but never scanned, in a section that had at least one scan that day.
WITH cur AS (
    SELECT DISTINCT ON (r.register_id, r.student_id) r.student_id, r.status, g.section_id
    FROM attendance_records r
    JOIN attendance_registers g ON g.tenant_id = r.tenant_id AND g.id = r.register_id
    WHERE r.tenant_id = sqlc.arg('tenant_id') AND g.status = 'submitted' AND g.day = sqlc.arg('day')
    ORDER BY r.register_id, r.student_id, r.version DESC
), arrivals AS (
    SELECT DISTINCT ON (student_id) student_id, scanned_at
    FROM gate_scans
    WHERE tenant_id = sqlc.arg('tenant_id') AND day = sqlc.arg('day')
    ORDER BY student_id, scanned_at, id
), scanned_sections AS (
    -- A section "uses the gate" if a student who is counted in its register was scanned in it. A student who
    -- was scanned and then moved (recorded as left) does not make their old section look scanned.
    SELECT DISTINCT s.section_id
    FROM gate_scans s
    JOIN cur ON cur.student_id = s.student_id AND cur.section_id = s.section_id AND cur.status <> 'left'
    WHERE s.tenant_id = sqlc.arg('tenant_id') AND s.day = sqlc.arg('day')
)
SELECT m.student_id, m.section_id, m.status, m.kind, m.gate_arrived_at
FROM (
    SELECT cur.student_id, cur.section_id, cur.status, a.scanned_at AS gate_arrived_at,
           CASE
               WHEN a.scanned_at IS NOT NULL AND cur.status = 'absent' THEN 'scanned_but_absent'
               WHEN a.scanned_at IS NULL AND cur.status IN ('present', 'late')
                    AND cur.section_id IN (SELECT section_id FROM scanned_sections) THEN 'present_but_not_scanned'
           END::text AS kind
    FROM cur
    LEFT JOIN arrivals a ON a.student_id = cur.student_id
) m
WHERE m.kind IS NOT NULL
ORDER BY m.section_id, m.kind, m.student_id;
