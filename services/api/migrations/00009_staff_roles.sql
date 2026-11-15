-- +goose Up
-- Built-in roles Accountant and Receptionist (REQ-IAM-005). Roles are code (iam/principal.go);
-- assignments are data. Widening the list is backward-compatible: every existing row stays valid.
ALTER TABLE role_assignments DROP CONSTRAINT role_assignments_role_check;
ALTER TABLE role_assignments ADD CONSTRAINT role_assignments_role_check
    CHECK (role IN ('owner', 'accountant', 'receptionist'));

-- +goose Down
ALTER TABLE role_assignments DROP CONSTRAINT role_assignments_role_check;
ALTER TABLE role_assignments ADD CONSTRAINT role_assignments_role_check CHECK (role IN ('owner'));
