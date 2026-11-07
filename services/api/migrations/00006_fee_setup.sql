-- +goose Up
-- Fee setup (slice 3b, ADR-0015): fee types, class plan versions and student override versions.
-- No money moves here and the ledger is not touched; challans (3c) read these tables.
-- Plan lines and overrides are EFFECTIVE-DATED, APPEND-ONLY VERSIONS: a change adds a row that
-- takes effect from a month, and earlier rows stay. A released challan can therefore never be
-- altered by a later change (BR-FEE-001), and the history is exact (REQ-FEE-001).

CREATE TABLE fee_types (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    code       text NOT NULL CHECK (code ~ '^[a-z][a-z0-9_]{1,29}$'),
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 60),
    kind       text NOT NULL CHECK (kind IN ('tuition', 'admission', 'registration', 'annual', 'transport',
                                             'lab', 'library', 'sports', 'fine', 'security_deposit', 'custom')),
    -- A security deposit is held for the family and is a liability, never income (BR-FEE-004).
    is_deposit boolean NOT NULL DEFAULT false,
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    CHECK (is_deposit = (kind = 'security_deposit')),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, code)
);

CREATE TABLE fee_plan_lines (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants (id),
    class_id       uuid NOT NULL,
    fee_type_id    uuid NOT NULL,
    -- Whole paisa. A removal version carries 0; every other version must be positive.
    amount         bigint NOT NULL CHECK (amount >= 0),
    frequency      text NOT NULL CHECK (frequency IN ('monthly', 'one_time', 'annual', 'ad_hoc')),
    effective_from date NOT NULL CHECK (EXTRACT(day FROM effective_from) = 1),
    removed        boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    created_by     uuid,
    CHECK (removed OR amount > 0),
    -- One version per class, fee type and month. A second is a conflict: pick a later month.
    UNIQUE (tenant_id, class_id, fee_type_id, effective_from),
    FOREIGN KEY (tenant_id, class_id) REFERENCES classes (tenant_id, id),
    FOREIGN KEY (tenant_id, fee_type_id) REFERENCES fee_types (tenant_id, id)
);

CREATE TABLE student_fee_overrides (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants (id),
    student_id     uuid NOT NULL,
    fee_type_id    uuid NOT NULL,
    amount         bigint NOT NULL CHECK (amount >= 0),     -- 0 is a full concession
    effective_from date NOT NULL CHECK (EXTRACT(day FROM effective_from) = 1),
    -- A "cleared" version sends the student back to the class plan amount from that month.
    cleared        boolean NOT NULL DEFAULT false,
    reason         text NOT NULL CHECK (length(reason) BETWEEN 3 AND 200),
    created_at     timestamptz NOT NULL DEFAULT now(),
    created_by     uuid,
    UNIQUE (tenant_id, student_id, fee_type_id, effective_from),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id),
    FOREIGN KEY (tenant_id, fee_type_id) REFERENCES fee_types (tenant_id, id)
);

-- Versions are append-only for every role (forbid_mutation is from 00001).
CREATE TRIGGER fee_plan_lines_append_only BEFORE UPDATE OR DELETE ON fee_plan_lines
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER student_fee_overrides_append_only BEFORE UPDATE OR DELETE ON student_fee_overrides
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER fee_types_no_delete BEFORE DELETE ON fee_types
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Row-level security, with the NULLIF form from 00003.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['fee_types', 'fee_plan_lines', 'student_fee_overrides']
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

-- Grants. Fee types may change only their status (archive); plan lines and overrides are insert-only.
GRANT SELECT, INSERT ON fee_types, fee_plan_lines, student_fee_overrides TO app_rt;
GRANT UPDATE (status) ON fee_types TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS student_fee_overrides;
DROP TABLE IF EXISTS fee_plan_lines;
DROP TABLE IF EXISTS fee_types;
