-- +goose Up
-- Fix forward (00001 is applied, so it is not edited).
-- After a pooled connection has run a tenant transaction, set_config(..., true) reverts the
-- setting to '' rather than NULL. Casting '' to uuid raised an error instead of matching no
-- rows. NULLIF makes "no tenant" always NULL, so no row matches and the default stays fail-closed.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['memberships', 'role_assignments', 'api_tokens', 'tenant_counters',
                             'families', 'guardians', 'students', 'guardian_links', 'audit_log']
    LOOP
        EXECUTE format('DROP POLICY tenant_isolation ON %I', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %I
               USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)
               WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['memberships', 'role_assignments', 'api_tokens', 'tenant_counters',
                             'families', 'guardians', 'students', 'guardian_links', 'audit_log']
    LOOP
        EXECUTE format('DROP POLICY tenant_isolation ON %I', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %I
               USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid)
               WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', t);
    END LOOP;
END
$$;
-- +goose StatementEnd
