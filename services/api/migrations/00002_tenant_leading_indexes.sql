-- +goose Up
-- Every tenant table gets an index that starts with tenant_id (sql-migrations rule).
-- These two tables were missed in 00001; added forward-only, not by editing 00001.
CREATE INDEX role_assignments_tenant_idx ON role_assignments (tenant_id, membership_id);
CREATE INDEX api_tokens_tenant_idx ON api_tokens (tenant_id, membership_id);

-- +goose Down
DROP INDEX IF EXISTS api_tokens_tenant_idx;
DROP INDEX IF EXISTS role_assignments_tenant_idx;
