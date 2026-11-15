-- +goose Up
-- Challans (slice 3c, ADR-0017): a monthly voucher per student, drafted, reviewed, then released.
-- A draft posts nothing. Release assigns the numbers and posts the receivable (ledger.Post) in the
-- same transaction. After release a challan is read-only for every role (BR-FEE-001): a trigger
-- refuses the update, the column grant is the second line, and the lines are append-only.

CREATE TABLE challans (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants (id),
    student_id  uuid NOT NULL,
    family_id   uuid NOT NULL,
    month       date NOT NULL CHECK (EXTRACT(day FROM month) = 1),
    due_date    date NOT NULL CHECK (due_date >= month),
    status      text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'released', 'void')),
    -- Whole paisa; the sum of the lines. A challan with nothing to bill is never created.
    total       bigint NOT NULL CHECK (total > 0),
    -- Set at release. challan_no is gapless per tenant (tenant_counters); consumer_no is 14 digits.
    challan_no  bigint,
    consumer_no text CHECK (consumer_no ~ '^[0-9]{14}$'),
    released_at timestamptz,
    released_by uuid,
    void_reason text CHECK (length(void_reason) BETWEEN 1 AND 300),
    voided_at   timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid,
    CHECK (status <> 'released' OR (challan_no IS NOT NULL AND consumer_no IS NOT NULL AND released_at IS NOT NULL)),
    CHECK (status = 'released' OR (challan_no IS NULL AND consumer_no IS NULL)),
    CHECK (status <> 'void' OR (void_reason IS NOT NULL AND voided_at IS NOT NULL)),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id),
    FOREIGN KEY (tenant_id, family_id) REFERENCES families (tenant_id, id)
);

-- One live challan per student and month: this is what makes Generate Fee safe to run twice.
-- A voided draft frees the slot.
CREATE UNIQUE INDEX challans_one_live_per_month ON challans (tenant_id, student_id, month) WHERE status <> 'void';
CREATE UNIQUE INDEX challans_number ON challans (tenant_id, challan_no) WHERE challan_no IS NOT NULL;
CREATE UNIQUE INDEX challans_consumer_no ON challans (tenant_id, consumer_no) WHERE consumer_no IS NOT NULL;
CREATE INDEX challans_by_month ON challans (tenant_id, month, status);
CREATE INDEX challans_by_created ON challans (tenant_id, created_at DESC, id DESC);

-- The lines are copied from the fee structure when the draft is made, and never change.
CREATE TABLE challan_lines (
    tenant_id     uuid NOT NULL REFERENCES tenants (id),
    challan_id    uuid NOT NULL,
    line_no       integer NOT NULL CHECK (line_no > 0),
    fee_type_id   uuid NOT NULL,
    fee_type_name text NOT NULL,
    kind          text NOT NULL,
    frequency     text NOT NULL CHECK (frequency IN ('monthly', 'one_time', 'annual', 'ad_hoc')),
    amount        bigint NOT NULL CHECK (amount > 0),
    PRIMARY KEY (tenant_id, challan_id, line_no),
    FOREIGN KEY (tenant_id, challan_id) REFERENCES challans (tenant_id, id),
    FOREIGN KEY (tenant_id, fee_type_id) REFERENCES fee_types (tenant_id, id)
);

CREATE TRIGGER challan_lines_append_only BEFORE UPDATE OR DELETE ON challan_lines
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER challans_no_delete BEFORE DELETE ON challans
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- A challan may change only while it is a draft, and never in the columns that define it.
-- +goose StatementBegin
CREATE FUNCTION challans_guard_update() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'draft' THEN
        RAISE EXCEPTION 'challan % is % and cannot be changed (BR-FEE-001)', OLD.id, OLD.status
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.student_id <> OLD.student_id
       OR NEW.family_id <> OLD.family_id OR NEW.month <> OLD.month OR NEW.due_date <> OLD.due_date
       OR NEW.total <> OLD.total THEN
        RAISE EXCEPTION 'challan % : identity, month, due date and total never change', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER challans_guard_update BEFORE UPDATE ON challans
    FOR EACH ROW EXECUTE FUNCTION challans_guard_update();

-- Row-level security, with the NULLIF form from 00003.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['challans', 'challan_lines']
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

GRANT SELECT, INSERT ON challans, challan_lines TO app_rt;
GRANT UPDATE (status, challan_no, consumer_no, released_at, released_by, void_reason, voided_at) ON challans TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS challan_lines;
DROP TABLE IF EXISTS challans;
DROP FUNCTION IF EXISTS challans_guard_update();
