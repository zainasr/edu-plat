-- +goose Up
-- DEC-011: business records are never hard-deleted. Migration 00001 granted DELETE on eight tables
-- the code never deletes from. The grant is a latent hole: a future bug or injected statement could
-- remove students, families, guardians, tokens or counters. Found by TestINV_TheAPIRoleCannotDeleteOrTruncateAnything.
-- Nothing in the application deletes rows (checked: no DELETE statement outside tests), so this
-- changes no behaviour. Records are archived, deactivated, revoked or reversed instead.
REVOKE DELETE ON api_tokens, families, guardian_links, guardians, memberships, role_assignments,
    students, tenant_counters FROM app_rt;

-- +goose Down
GRANT DELETE ON api_tokens, families, guardian_links, guardians, memberships, role_assignments,
    students, tenant_counters TO app_rt;
