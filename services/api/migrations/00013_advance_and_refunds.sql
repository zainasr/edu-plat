-- +goose Up
-- Advance and refunds (slice 5b, ADR-0021). A payment above what a family owes may hold the extra as
-- Advance (DEC-010: it is "against the next challan", not a free-floating credit). The advance is applied
-- to the family's challans when they are released, and what cannot be applied is refunded explicitly.
-- Everything is history: applications and refunds are append-only, and what is left of an advance is
-- worked out, never stored. Three guarantees live in the database, checked at commit:
--   * a payment's allocations plus its advance equal its amount;
--   * a challan is never paid more than its total (allocations plus applied advance);
--   * an advance is never spent (applied or refunded) beyond what the payment held.

ALTER TABLE payments ADD COLUMN advance bigint NOT NULL DEFAULT 0 CHECK (advance >= 0 AND advance <= amount);
CREATE INDEX payments_with_advance ON payments (tenant_id, family_id, created_at, id) WHERE advance > 0;

CREATE TABLE advance_applications (
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    payment_id uuid NOT NULL,
    challan_id uuid NOT NULL,
    amount     bigint NOT NULL CHECK (amount > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    PRIMARY KEY (tenant_id, payment_id, challan_id),
    FOREIGN KEY (tenant_id, payment_id) REFERENCES payments (tenant_id, id),
    FOREIGN KEY (tenant_id, challan_id) REFERENCES challans (tenant_id, id)
);
CREATE INDEX advance_applications_by_challan ON advance_applications (tenant_id, challan_id);

CREATE TABLE advance_refunds (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants (id),
    payment_id       uuid NOT NULL,
    amount           bigint NOT NULL CHECK (amount > 0),
    account_code     text NOT NULL CHECK (account_code IN ('1000', '1100')),
    reason           text NOT NULL CHECK (length(reason) BETWEEN 3 AND 200),
    journal_entry_id uuid NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, payment_id) REFERENCES payments (tenant_id, id),
    FOREIGN KEY (tenant_id, journal_entry_id) REFERENCES journal_entries (tenant_id, id)
);
CREATE INDEX advance_refunds_by_payment ON advance_refunds (tenant_id, payment_id);

CREATE TRIGGER advance_applications_append_only BEFORE UPDATE OR DELETE ON advance_applications
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER advance_refunds_append_only BEFORE UPDATE OR DELETE ON advance_refunds
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Guarantee 1: allocations plus advance equal the payment's amount.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION assert_payment_allocated() RETURNS trigger AS $$
DECLARE
    v_payment uuid;
    v_amount  bigint;
    v_advance bigint;
    v_sum     bigint;
BEGIN
    IF TG_TABLE_NAME = 'payments' THEN
        v_payment := NEW.id;
    ELSE
        v_payment := NEW.payment_id;
    END IF;
    SELECT amount, advance INTO v_amount, v_advance FROM payments WHERE tenant_id = NEW.tenant_id AND id = v_payment;
    SELECT COALESCE(sum(amount), 0) INTO v_sum FROM payment_allocations WHERE tenant_id = NEW.tenant_id AND payment_id = v_payment;
    IF v_sum + v_advance <> v_amount THEN
        RAISE EXCEPTION 'payment % is for % but its allocations (%) and advance (%) do not add up', v_payment, v_amount, v_sum, v_advance
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Guarantee 2: a released challan of the paying family is never paid beyond its total, counting both
-- payment allocations and advance applied to it. Runs for allocations and for advance applications.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION assert_challan_not_overpaid() RETURNS trigger AS $$
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
    SELECT COALESCE((SELECT sum(a.amount) FROM payment_allocations a
                       JOIN payments p ON p.tenant_id = a.tenant_id AND p.id = a.payment_id
                      WHERE a.tenant_id = NEW.tenant_id AND a.challan_id = NEW.challan_id AND p.status = 'recorded'), 0)
         + COALESCE((SELECT sum(x.amount) FROM advance_applications x
                      WHERE x.tenant_id = NEW.tenant_id AND x.challan_id = NEW.challan_id), 0)
      INTO v_paid;
    IF v_paid > v_total THEN
        RAISE EXCEPTION 'challan % has % paid against a total of %', NEW.challan_id, v_paid, v_total
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER advance_applications_not_overpaid AFTER INSERT ON advance_applications
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_challan_not_overpaid();

-- Guarantee 3: an advance is never spent beyond what the payment held, and only a payment that stands holds one.
-- +goose StatementBegin
CREATE FUNCTION assert_advance_not_overspent() RETURNS trigger AS $$
DECLARE
    v_advance bigint;
    v_status  text;
    v_used    bigint;
BEGIN
    SELECT advance, status INTO v_advance, v_status FROM payments WHERE tenant_id = NEW.tenant_id AND id = NEW.payment_id;
    IF v_status <> 'recorded' THEN
        RAISE EXCEPTION 'payment % is % and holds no advance', NEW.payment_id, v_status USING ERRCODE = 'check_violation';
    END IF;
    SELECT COALESCE((SELECT sum(amount) FROM advance_applications WHERE tenant_id = NEW.tenant_id AND payment_id = NEW.payment_id), 0)
         + COALESCE((SELECT sum(amount) FROM advance_refunds WHERE tenant_id = NEW.tenant_id AND payment_id = NEW.payment_id), 0)
      INTO v_used;
    IF v_used > v_advance THEN
        RAISE EXCEPTION 'payment % held % of advance but % has been spent', NEW.payment_id, v_advance, v_used
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NULL;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER advance_applications_not_overspent AFTER INSERT ON advance_applications
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_advance_not_overspent();
CREATE CONSTRAINT TRIGGER advance_refunds_not_overspent AFTER INSERT ON advance_refunds
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assert_advance_not_overspent();

-- A payment changes once, from recorded to voided; its amounts never change, and it cannot be voided
-- once any of its advance has been applied or refunded (that money has already moved on).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION payments_guard_update() RETURNS trigger AS $$
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
       OR NEW.advance <> OLD.advance
       OR NEW.method <> OLD.method OR NEW.account_code <> OLD.account_code
       OR NEW.reference IS DISTINCT FROM OLD.reference OR NEW.received_on <> OLD.received_on
       OR NEW.journal_entry_id <> OLD.journal_entry_id THEN
        RAISE EXCEPTION 'payment % : only the void can be recorded', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    IF EXISTS (SELECT 1 FROM advance_applications WHERE tenant_id = OLD.tenant_id AND payment_id = OLD.id)
       OR EXISTS (SELECT 1 FROM advance_refunds WHERE tenant_id = OLD.tenant_id AND payment_id = OLD.id) THEN
        RAISE EXCEPTION 'payment % has had advance applied or refunded and cannot be voided', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Row-level security, with the NULLIF form from 00003.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['advance_applications', 'advance_refunds']
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

GRANT SELECT, INSERT ON advance_applications, advance_refunds TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS advance_refunds;
DROP TABLE IF EXISTS advance_applications;
DROP FUNCTION IF EXISTS assert_advance_not_overspent();
-- Put back the 00012 versions of the functions that this migration replaced (they do not know about advance).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION assert_payment_allocated() RETURNS trigger AS $$
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
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION assert_challan_not_overpaid() RETURNS trigger AS $$
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
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION payments_guard_update() RETURNS trigger AS $$
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
ALTER TABLE payments DROP COLUMN IF EXISTS advance;
