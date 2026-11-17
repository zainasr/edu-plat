-- +goose Up
-- School calendar and attendance registers (slice 6a, ADR-0022).
-- The calendar (REQ-ORG-011) says which days are school days. A register (REQ-ATT-001) is one section's
-- attendance for one day; its records are VERSIONS: every mark and every correction adds a row and nothing
-- is overwritten (BR-ATT-002). The database enforces the lifecycle: marks only while a register is open,
-- corrections only after it is submitted, versions without gaps.

-- A new built-in role for the attendance desk (owner decision 2026-11-17; teachers come with the staff module).
ALTER TABLE role_assignments DROP CONSTRAINT role_assignments_role_check;
ALTER TABLE role_assignments ADD CONSTRAINT role_assignments_role_check
    CHECK (role IN ('owner', 'accountant', 'receptionist', 'attendance_clerk'));

-- ---- calendar -------------------------------------------------------------------------

CREATE TABLE calendar_settings (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants (id),
    -- 0 = Sunday ... 6 = Saturday. Fewer than seven, or no day would ever be a school day.
    weekly_off_days smallint[] NOT NULL DEFAULT '{0}'
        CHECK (cardinality(weekly_off_days) <= 6 AND weekly_off_days <@ ARRAY[0, 1, 2, 3, 4, 5, 6]::smallint[]),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      uuid,
    UNIQUE (tenant_id),
    UNIQUE (tenant_id, id)
);

CREATE TABLE calendar_days (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    day        date NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('holiday', 'working')),
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'removed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    removed_at timestamptz,
    removed_by uuid,
    CHECK (status = 'removed' OR removed_at IS NULL),
    CHECK (status <> 'removed' OR removed_at IS NOT NULL),
    UNIQUE (tenant_id, id)
);
-- One active entry per date. A removed entry frees the date.
CREATE UNIQUE INDEX calendar_days_one_active ON calendar_days (tenant_id, day) WHERE status = 'active';

-- A calendar entry is only ever removed, never edited or deleted.
-- +goose StatementBegin
CREATE FUNCTION calendar_days_guard_update() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'active' THEN
        RAISE EXCEPTION 'calendar day % is removed and cannot be changed', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.status <> 'removed' OR NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.day <> OLD.day
       OR NEW.kind <> OLD.kind OR NEW.name <> OLD.name THEN
        RAISE EXCEPTION 'calendar day % can only be removed, never edited', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER calendar_days_guard_update BEFORE UPDATE ON calendar_days
    FOR EACH ROW EXECUTE FUNCTION calendar_days_guard_update();
CREATE TRIGGER calendar_days_no_delete BEFORE DELETE ON calendar_days
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- ---- attendance -----------------------------------------------------------------------

CREATE TABLE attendance_settings (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              uuid NOT NULL REFERENCES tenants (id),
    register_due_time      time NOT NULL DEFAULT '10:00',
    edit_window_days       integer NOT NULL DEFAULT 7 CHECK (edit_window_days BETWEEN 1 AND 90),
    low_attendance_percent integer NOT NULL DEFAULT 75 CHECK (low_attendance_percent BETWEEN 1 AND 100),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    updated_by             uuid,
    UNIQUE (tenant_id),
    UNIQUE (tenant_id, id)
);

CREATE TABLE attendance_registers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants (id),
    section_id   uuid NOT NULL,
    day          date NOT NULL,
    status       text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'submitted')),
    submitted_at timestamptz,
    submitted_by uuid,
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   uuid,
    CHECK ((status = 'submitted') = (submitted_at IS NOT NULL)),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, section_id, day),
    FOREIGN KEY (tenant_id, section_id) REFERENCES sections (tenant_id, id)
);
CREATE INDEX attendance_registers_by_day ON attendance_registers (tenant_id, day, status);

-- A register is submitted once and changes in no other way (BR-ATT-005).
-- +goose StatementBegin
CREATE FUNCTION attendance_registers_guard_update() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'open' OR NEW.status <> 'submitted' THEN
        RAISE EXCEPTION 'register % can only go from open to submitted, once', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.section_id <> OLD.section_id OR NEW.day <> OLD.day THEN
        RAISE EXCEPTION 'register % : only the submission can be recorded', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER attendance_registers_guard_update BEFORE UPDATE ON attendance_registers
    FOR EACH ROW EXECUTE FUNCTION attendance_registers_guard_update();
CREATE TRIGGER attendance_registers_no_delete BEFORE DELETE ON attendance_registers
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Every mark and every correction is a new version of a student's record in a register.
CREATE TABLE attendance_records (
    tenant_id   uuid NOT NULL REFERENCES tenants (id),
    register_id uuid NOT NULL,
    student_id  uuid NOT NULL,
    version     integer NOT NULL CHECK (version >= 1),
    -- 'left' is written only when a register is submitted, for a student who is no longer enrolled in the
    -- section that day (moved or withdrew): they are not counted anywhere.
    status      text NOT NULL CHECK (status IN ('present', 'absent', 'late', 'leave', 'left')),
    method      text NOT NULL CHECK (method IN ('prefill', 'manual', 'correction')),
    reason      text CHECK (length(reason) BETWEEN 3 AND 200),
    changed_by  uuid,
    changed_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (method <> 'correction' OR reason IS NOT NULL),
    CHECK (method <> 'correction' OR version > 1),
    PRIMARY KEY (tenant_id, register_id, student_id, version),
    FOREIGN KEY (tenant_id, register_id) REFERENCES attendance_registers (tenant_id, id),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id)
);
CREATE INDEX attendance_records_by_student ON attendance_records (tenant_id, student_id, register_id);

CREATE TRIGGER attendance_records_append_only BEFORE UPDATE OR DELETE ON attendance_records
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- The lifecycle, enforced where the data lives: versions count 1, 2, 3 with no gaps; marks and the first
-- entry only while the register is open; corrections only after it is submitted.
-- +goose StatementBegin
CREATE FUNCTION attendance_records_lifecycle() RETURNS trigger AS $$
DECLARE
    v_status text;
    v_last   integer;
BEGIN
    -- FOR SHARE: a concurrent submit (an UPDATE of this row) must wait for us, or we for it, so a mark cannot
    -- land in a register that was submitted a moment earlier.
    SELECT status INTO v_status FROM attendance_registers WHERE tenant_id = NEW.tenant_id AND id = NEW.register_id FOR SHARE;
    SELECT COALESCE(max(version), 0) INTO v_last FROM attendance_records
     WHERE tenant_id = NEW.tenant_id AND register_id = NEW.register_id AND student_id = NEW.student_id;
    IF NEW.version <> v_last + 1 THEN
        RAISE EXCEPTION 'attendance version % is not next after %', NEW.version, v_last USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.method = 'correction' AND v_status <> 'submitted' THEN
        RAISE EXCEPTION 'only a submitted register is corrected; mark an open one' USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.method <> 'correction' AND v_status <> 'open' THEN
        RAISE EXCEPTION 'a submitted register changes only by correction' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER attendance_records_lifecycle BEFORE INSERT ON attendance_records
    FOR EACH ROW EXECUTE FUNCTION attendance_records_lifecycle();

-- Row-level security, with the NULLIF form from 00003.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['calendar_settings', 'calendar_days', 'attendance_settings', 'attendance_registers', 'attendance_records']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %I
               USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)
               WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    END LOOP;
END
$$;
-- +goose StatementEnd

GRANT SELECT, INSERT ON calendar_settings, calendar_days, attendance_settings, attendance_registers, attendance_records TO app_rt;
GRANT UPDATE (weekly_off_days, updated_at, updated_by) ON calendar_settings TO app_rt;
GRANT UPDATE (status, removed_at, removed_by) ON calendar_days TO app_rt;
GRANT UPDATE (register_due_time, edit_window_days, low_attendance_percent, updated_at, updated_by) ON attendance_settings TO app_rt;
GRANT UPDATE (status, submitted_at, submitted_by) ON attendance_registers TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS attendance_records;
DROP TABLE IF EXISTS attendance_registers;
DROP TABLE IF EXISTS attendance_settings;
DROP TABLE IF EXISTS calendar_days;
DROP TABLE IF EXISTS calendar_settings;
DROP FUNCTION IF EXISTS attendance_records_lifecycle();
DROP FUNCTION IF EXISTS attendance_registers_guard_update();
DROP FUNCTION IF EXISTS calendar_days_guard_update();
-- The widened role list is left as it is: narrowing it would fail for any school that already has a clerk.
