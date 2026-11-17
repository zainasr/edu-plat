-- +goose Up
-- Payments at the counter (slice 5a, ADR-0020). A payment belongs to a family, is split across that
-- family's released challans, and has a gapless receipt number. Challans stay immutable (BR-FEE-001): what
-- is paid on a challan is worked out from the allocations of payments that stand, never stored on it.
-- Two guarantees live in the database, checked at commit like the ledger's balance:
--   * a payment's allocations add up to its amount, and
--   * no challan is paid more than its total.

CREATE TABLE payments (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants (id),
    family_id        uuid NOT NULL,
    fiscal_year      integer NOT NULL CHECK (fiscal_year BETWEEN 2000 AND 2200),
    receipt_no       bigint NOT NULL CHECK (receipt_no > 0),
    amount           bigint NOT NULL CHECK (amount > 0),       -- whole paisa
    method           text NOT NULL CHECK (method IN ('cash', 'bank_transfer', 'cheque')),
    account_code     text NOT NULL CHECK (account_code IN ('1000', '1100')),
    reference        text CHECK (length(reference) BETWEEN 1 AND 100),
    received_on      date NOT NULL,
    status           text NOT NULL DEFAULT 'recorded' CHECK (status IN ('recorded', 'voided')),
    journal_entry_id uuid NOT NULL,
    void_reason      text CHECK (length(void_reason) BETWEEN 3 AND 200),
    voided_at        timestamptz,
    voided_by        uuid,
    void_entry_id    uuid,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid,
    CHECK (status <> 'voided' OR (void_reason IS NOT NULL AND voided_at IS NOT NULL AND void_entry_id IS NOT NULL)),
    CHECK (status = 'voided' OR (void_reason IS NULL AND void_entry_id IS NULL)),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, fiscal_year, receipt_no),
    FOREIGN KEY (tenant_id, family_id) REFERENCES families (tenant_id, id),
    FOREIGN KEY (tenant_id, journal_entry_id) REFERENCES journal_entries (tenant_id, id),
    FOREIGN KEY (tenant_id, void_entry_id) REFERENCES journal_entries (tenant_id, id)
);
CREATE INDEX payments_by_family ON payments (tenant_id, family_id, created_at DESC);
CREATE INDEX payments_by_created ON payments (tenant_id, created_at DESC, id DESC);

CREATE TABLE payment_allocations (
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    payment_id uuid NOT NULL,
    challan_id uuid NOT NULL,
    amount     bigint NOT NULL CHECK (amount > 0),
    PRIMARY KEY (tenant_id, payment_id, challan_id),
    FOREIGN KEY (tenant_id, payment_id) REFERENCES payments (tenant_id, id),
    FOREIGN KEY (tenant_id, challan_id) REFERENCES challans (tenant_id, id)
);
CREATE INDEX payment_allocations_by_challan ON payment_allocations (tenant_id, challan_id);

-- Allocations are history: never changed, never deleted, for every role.
CREATE TRIGGER payment_allocations_append_only BEFORE UPDATE OR DELETE ON payment_allocations
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER payments_no_delete BEFORE DELETE ON payments
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- A payment changes once, from recorded to voided, and only in the void columns (BR-FEE-003).
-- +goose StatementBegin
CREATE FUNCTION payments_guard_update() RETURNS trigger AS $$
BEGIN
    IF OLD.status <> 'recorded' THEN
        RAISE EXCEPTION 'payment % is % and cannot be changed (BR-FEE-003)', OLD.id, OLD.status
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.status <> 'voided' THEN
        RAISE EXCEPTION 'payment % can only be voided, never edited', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.family_id <> OLD.family_id
       OR NEW.fiscal_year <> OLD.fiscal_year OR NEW.receipt_no <> OLD.receipt_no OR NEW.amount <> OLD.amount
       OR NEW.method <> OLD.method OR NEW.account_code <> OLD.account_code
       OR NEW.reference IS DISTINCT FROM OLD.reference OR NEW.received_on <> OLD.received_on
       OR NEW.journal_entry_id <> OLD.journal_entry_id THEN
        RAISE EXCEPTION 'payment % : only the void can be recorded', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER payments_guard_update BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION payments_guard_update();

-- Guarantee 1: a payment's allocations add up to its amount. Deferred to commit, because the payment is
-- inserted first and its allocations after it.
-- +goose StatementBegin
CREATE FUNCTION assert_payment_allocated() RETURNS trigger AS $$
DECLARE
    v_payment uuid;
    v_amount  bigint;
    v_sum     bigint;
BEGIN
    IF TG_TABLE_NAME = 'payments' THEN
        v_payment := NEW.id;
    ELSE
        v_payment := NEW.payment_id;
    END IF;
    SELECT amount INTO v_amount FROM payments WHERE tenant_id = NEW.tenant_id AND id = v_payment;
    SELECT COALESCE(sum(amount), 0) INTO v_sum FROM payment_allocations WHERE tenant_id = NEW.tenant_id AND payment_id = v_payment;
    IF v_sum <> v_amount THEN
        RAISE EXCEPTION 'payment % is for % but its allocations add up to %', v_payment, v_amount, v_sum
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER payments_allocated AFTER INSERT ON payments
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_payment_allocated();
CREATE CONSTRAINT TRIGGER payment_allocations_allocated AFTER INSERT ON payment_allocations
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_payment_allocated();

-- Guarantee 2: a challan is released, belongs to the paying family, and is never paid more than its total by
-- the payments that stand. (Concurrent payments are serialised by the application's locks on the family's
-- challans; this is the backstop that refuses a commit if that ever failed.)
-- +goose StatementBegin
CREATE FUNCTION assert_challan_not_overpaid() RETURNS trigger AS $$
DECLARE
    v_total          bigint;
    v_status         text;
    v_challan_family uuid;
    v_payment_family uuid;
    v_paid           bigint;
BEGIN
    SELECT total, status, family_id INTO v_total, v_status, v_challan_family
    FROM challans WHERE tenant_id = NEW.tenant_id AND id = NEW.challan_id;
    IF v_status <> 'released' THEN
        RAISE EXCEPTION 'challan % is % and cannot be paid', NEW.challan_id, v_status USING ERRCODE = 'check_violation';
    END IF;
    SELECT family_id INTO v_payment_family FROM payments WHERE tenant_id = NEW.tenant_id AND id = NEW.payment_id;
    IF v_payment_family <> v_challan_family THEN
        RAISE EXCEPTION 'payment % is from another family than challan %', NEW.payment_id, NEW.challan_id
            USING ERRCODE = 'check_violation';
    END IF;
    SELECT COALESCE(sum(a.amount), 0) INTO v_paid
    FROM payment_allocations a
    JOIN payments p ON p.tenant_id = a.tenant_id AND p.id = a.payment_id
    WHERE a.tenant_id = NEW.tenant_id AND a.challan_id = NEW.challan_id AND p.status = 'recorded';
    IF v_paid > v_total THEN
        RAISE EXCEPTION 'challan % has % paid against a total of %', NEW.challan_id, v_paid, v_total
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER payment_allocations_not_overpaid AFTER INSERT ON payment_allocations
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_challan_not_overpaid();

-- Row-level security, with the NULLIF form from 00003.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['payments', 'payment_allocations']
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

GRANT SELECT, INSERT ON payments, payment_allocations TO app_rt;
GRANT UPDATE (status, void_reason, voided_at, voided_by, void_entry_id) ON payments TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS payment_allocations;
DROP TABLE IF EXISTS payments;
DROP FUNCTION IF EXISTS assert_challan_not_overpaid();
DROP FUNCTION IF EXISTS assert_payment_allocated();
DROP FUNCTION IF EXISTS payments_guard_update();
