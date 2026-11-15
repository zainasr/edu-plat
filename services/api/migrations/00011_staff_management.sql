-- +goose Up
-- Staff and roles (slice 4b, ADR-0019): revocable role assignments, deactivation, a per-school display
-- name, and one narrow function that adds a person by mobile number.

-- 1. Role assignments are revoked, never deleted (REQ-IAM-006, DEC-011). A person may hold a role again
--    after it was revoked, so uniqueness applies to live assignments only.
ALTER TABLE role_assignments ADD COLUMN revoked_at timestamptz;
ALTER TABLE role_assignments ADD COLUMN revoked_by uuid;
ALTER TABLE role_assignments DROP CONSTRAINT role_assignments_membership_id_role_key;
CREATE UNIQUE INDEX role_assignments_live ON role_assignments (tenant_id, membership_id, role) WHERE revoked_at IS NULL;

-- A revoked assignment never changes again, and only the revocation columns may be written.
-- +goose StatementBegin
CREATE FUNCTION role_assignments_guard_update() RETURNS trigger AS $$
BEGIN
    IF OLD.revoked_at IS NOT NULL THEN
        RAISE EXCEPTION 'role assignment % is revoked and cannot be changed', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.id <> OLD.id OR NEW.tenant_id <> OLD.tenant_id OR NEW.membership_id <> OLD.membership_id
       OR NEW.role <> OLD.role OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'role assignment % : only the revocation can be recorded', OLD.id USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER role_assignments_guard_update BEFORE UPDATE ON role_assignments
    FOR EACH ROW EXECUTE FUNCTION role_assignments_guard_update();
REVOKE UPDATE ON role_assignments FROM app_rt;
GRANT UPDATE (revoked_at, revoked_by) ON role_assignments TO app_rt;

-- 2. Memberships: who switched someone off and when, and the name this school uses for the person.
--    The display name is the school's own, so adding a number that already exists elsewhere does not
--    reveal the name another school holds for it.
ALTER TABLE memberships ADD COLUMN display_name text CHECK (length(display_name) BETWEEN 1 AND 80);
ALTER TABLE memberships ADD COLUMN deactivated_at timestamptz;
ALTER TABLE memberships ADD COLUMN deactivated_by uuid;
REVOKE UPDATE ON memberships FROM app_rt;
GRANT UPDATE (status, deactivated_at, deactivated_by) ON memberships TO app_rt;

-- 3. Adding a person. The API role cannot write `users` (a table shared by every school), so one
--    narrow function does the only thing needed: add the person if the mobile is new, else return the
--    existing id without changing anything (ADR-0012 pattern).
-- +goose StatementBegin
CREATE FUNCTION ensure_user(p_mobile text, p_full_name text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
    v_id uuid;
BEGIN
    INSERT INTO users (mobile, full_name) VALUES (p_mobile, p_full_name)
    ON CONFLICT (mobile) DO NOTHING
    RETURNING id INTO v_id;
    IF v_id IS NULL THEN
        SELECT id INTO v_id FROM users WHERE mobile = p_mobile;
    END IF;
    RETURN v_id;
END
$$;
-- +goose StatementEnd
REVOKE EXECUTE ON FUNCTION ensure_user(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ensure_user(text, text) TO app_rt;

-- +goose Down
DROP FUNCTION IF EXISTS ensure_user(text, text);
GRANT UPDATE ON memberships TO app_rt;
ALTER TABLE memberships DROP COLUMN IF EXISTS deactivated_by;
ALTER TABLE memberships DROP COLUMN IF EXISTS deactivated_at;
ALTER TABLE memberships DROP COLUMN IF EXISTS display_name;
GRANT UPDATE ON role_assignments TO app_rt;
DROP TRIGGER IF EXISTS role_assignments_guard_update ON role_assignments;
DROP FUNCTION IF EXISTS role_assignments_guard_update();
DROP INDEX IF EXISTS role_assignments_live;
ALTER TABLE role_assignments ADD CONSTRAINT role_assignments_membership_id_role_key UNIQUE (membership_id, role);
ALTER TABLE role_assignments DROP COLUMN IF EXISTS revoked_by;
ALTER TABLE role_assignments DROP COLUMN IF EXISTS revoked_at;
