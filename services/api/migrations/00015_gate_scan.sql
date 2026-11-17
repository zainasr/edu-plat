-- +goose Up
-- The gate scan and student ID cards (slice 6b, ADR-0023). A student has one active card; its QR holds a
-- signed code the server can regenerate, so only the card id is stored. Scans are an append-only log kept
-- beside the class register (REQ-ATT-017): the register stays the official record, and disagreements are
-- listed for a person to review.

-- Per-shift start times and the grace after which a scan counts as late (REQ-ATT-002).
ALTER TABLE attendance_settings ADD COLUMN morning_start time NOT NULL DEFAULT '08:00';
ALTER TABLE attendance_settings ADD COLUMN evening_start time NOT NULL DEFAULT '13:00';
ALTER TABLE attendance_settings ADD COLUMN late_after_minutes integer NOT NULL DEFAULT 10 CHECK (late_after_minutes BETWEEN 0 AND 120);
GRANT UPDATE (morning_start, evening_start, late_after_minutes) ON attendance_settings TO app_rt;

CREATE TABLE student_cards (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants (id),
    student_id      uuid NOT NULL,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    issued_at       timestamptz NOT NULL DEFAULT now(),
    issued_by       uuid,
    disabled_at     timestamptz,
    disabled_by     uuid,
    disabled_reason text CHECK (length(disabled_reason) BETWEEN 3 AND 200),
    CHECK (status = 'disabled' OR (disabled_at IS NULL AND disabled_reason IS NULL)),
    CHECK (status <> 'disabled' OR (disabled_at IS NOT NULL AND disabled_reason IS NOT NULL)),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id)
);
-- One active card per student: issuing a new one disables the old one first.
CREATE UNIQUE INDEX student_cards_one_active ON student_cards (tenant_id, student_id) WHERE status = 'active';

-- A card only ever goes from active to disabled, with a reason, and is never deleted.
-- +goose StatementBegin
CREATE FUNCTION student_cards_guard_update() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'active' OR NEW.status <> 'disabled' THEN
        RAISE EXCEPTION 'card % can only go from active to disabled, once', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.student_id <> OLD.student_id OR NEW.issued_at <> OLD.issued_at THEN
        RAISE EXCEPTION 'card % : only the disabling can be recorded', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER student_cards_guard_update BEFORE UPDATE ON student_cards
    FOR EACH ROW EXECUTE FUNCTION student_cards_guard_update();
CREATE TRIGGER student_cards_no_delete BEFORE DELETE ON student_cards
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE gate_scans (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants (id),
    student_id     uuid NOT NULL,
    card_id        uuid NOT NULL,
    section_id     uuid NOT NULL,
    day            date NOT NULL,
    scanned_at     timestamptz NOT NULL,
    status         text NOT NULL CHECK (status IN ('present', 'late')),
    scanned_by     uuid NOT NULL,
    -- Chosen by the scanning computer so that an upload repeated after a lost connection is one scan.
    client_scan_id uuid,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id),
    FOREIGN KEY (tenant_id, card_id) REFERENCES student_cards (tenant_id, id),
    FOREIGN KEY (tenant_id, section_id) REFERENCES sections (tenant_id, id)
);
CREATE UNIQUE INDEX gate_scans_client_id ON gate_scans (tenant_id, client_scan_id) WHERE client_scan_id IS NOT NULL;
CREATE INDEX gate_scans_by_day ON gate_scans (tenant_id, day, section_id, scanned_at);
CREATE INDEX gate_scans_by_student ON gate_scans (tenant_id, student_id, day, scanned_at);
CREATE TRIGGER gate_scans_append_only BEFORE UPDATE OR DELETE ON gate_scans
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Row-level security, with the NULLIF form from 00003.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['student_cards', 'gate_scans']
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

GRANT SELECT, INSERT ON student_cards, gate_scans TO app_rt;
GRANT UPDATE (status, disabled_at, disabled_by, disabled_reason) ON student_cards TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS gate_scans;
DROP TABLE IF EXISTS student_cards;
DROP FUNCTION IF EXISTS student_cards_guard_update();
ALTER TABLE attendance_settings DROP COLUMN IF EXISTS late_after_minutes;
ALTER TABLE attendance_settings DROP COLUMN IF EXISTS evening_start;
ALTER TABLE attendance_settings DROP COLUMN IF EXISTS morning_start;
