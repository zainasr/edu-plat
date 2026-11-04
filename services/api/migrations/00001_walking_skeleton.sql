-- +goose Up
-- Walking skeleton schema (ADR-0011). Applied as the migration owner (role "app").
-- The API connects as "app_rt": not the owner, not a superuser, no BYPASSRLS,
-- so every policy below applies to it.

-- ---------------------------------------------------------------------------
-- Global tables (not tenant-owned): the tenant directory, identity and OTP.
-- ---------------------------------------------------------------------------
CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]{3,40}$'),
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- One identity per mobile number (REQ-IAM-001). Created by bootstrap or an admin.
CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mobile     text NOT NULL UNIQUE CHECK (mobile ~ '^\+923[0-9]{9}$'),
    full_name  text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Codes are stored as HMAC-SHA256, never in clear (ADR-0011 §4).
CREATE TABLE otp_challenges (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mobile      text NOT NULL CHECK (mobile ~ '^\+923[0-9]{9}$'),
    code_hash   bytea NOT NULL,
    expires_at  timestamptz NOT NULL,
    attempts    integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX otp_challenges_mobile_idx ON otp_challenges (mobile, created_at DESC);

-- ---------------------------------------------------------------------------
-- Tenant-scoped tables: tenant_id, composite keys, RLS policy.
-- ---------------------------------------------------------------------------
CREATE TABLE memberships (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    user_id    uuid NOT NULL REFERENCES users (id),
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deactivated')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    UNIQUE (tenant_id, user_id),
    UNIQUE (tenant_id, id)
);

-- A membership holds many role assignments; permissions are their union (REQ-IAM-002).
CREATE TABLE role_assignments (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants (id),
    membership_id uuid NOT NULL,
    role          text NOT NULL CHECK (role IN ('owner')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    created_by    uuid,
    UNIQUE (membership_id, role),
    FOREIGN KEY (tenant_id, membership_id) REFERENCES memberships (tenant_id, id)
);

-- Sessions (kind 'session') and personal access tokens (kind 'pat') share this table.
-- Only the SHA-256 of the full token is stored.
CREATE TABLE api_tokens (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants (id),
    membership_id uuid NOT NULL,
    user_id       uuid NOT NULL REFERENCES users (id),
    kind          text NOT NULL CHECK (kind IN ('session', 'pat')),
    name          text NOT NULL,
    token_hash    bytea NOT NULL UNIQUE,
    expires_at    timestamptz NOT NULL,
    revoked_at    timestamptz,
    last_used_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, membership_id) REFERENCES memberships (tenant_id, id)
);

-- Atomic per-tenant counters for registration and family numbers.
CREATE TABLE tenant_counters (
    tenant_id uuid NOT NULL REFERENCES tenants (id),
    name      text NOT NULL,
    value     bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, name)
);

CREATE TABLE families (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    number     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    UNIQUE (tenant_id, number),
    UNIQUE (tenant_id, id)
);

-- One guardian row per mobile per tenant: the same person is found by mobile (REQ-STU-004).
CREATE TABLE guardians (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    family_id  uuid NOT NULL,
    full_name  text NOT NULL,
    mobile     text NOT NULL CHECK (mobile ~ '^\+923[0-9]{9}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    UNIQUE (tenant_id, mobile),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, family_id) REFERENCES families (tenant_id, id)
);

CREATE TABLE students (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants (id),
    family_id       uuid NOT NULL,
    registration_no text NOT NULL,
    full_name       text NOT NULL,
    father_name     text NOT NULL,
    gender          text NOT NULL CHECK (gender IN ('male', 'female', 'other')),
    date_of_birth   date NOT NULL,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      uuid,
    UNIQUE (tenant_id, registration_no),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, family_id) REFERENCES families (tenant_id, id)
);
CREATE INDEX students_tenant_recent_idx ON students (tenant_id, created_at DESC, id DESC);

-- Guardian-to-student link with the flags of REQ-STU-015.
CREATE TABLE guardian_links (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              uuid NOT NULL REFERENCES tenants (id),
    student_id             uuid NOT NULL,
    guardian_id            uuid NOT NULL,
    relationship           text NOT NULL CHECK (relationship IN
                             ('father', 'mother', 'grandfather', 'grandmother', 'uncle', 'aunt', 'brother', 'sister', 'other')),
    is_primary             boolean NOT NULL,
    is_fee_payer           boolean NOT NULL,
    receives_notifications boolean NOT NULL,
    emergency_contact      boolean NOT NULL,
    access                 text NOT NULL CHECK (access IN ('full', 'view_only', 'none')),
    created_at             timestamptz NOT NULL DEFAULT now(),
    created_by             uuid,
    UNIQUE (tenant_id, student_id, guardian_id),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id),
    FOREIGN KEY (tenant_id, guardian_id) REFERENCES guardians (tenant_id, id)
);
-- At most one primary and one fee payer per student (BR-STU-005, BR-STU-006).
CREATE UNIQUE INDEX guardian_links_one_primary ON guardian_links (student_id) WHERE is_primary;
CREATE UNIQUE INDEX guardian_links_one_fee_payer ON guardian_links (student_id) WHERE is_fee_payer;

-- Append-only audit log (REQ-IAM-020, REQ-API-017). Source is 'api' or 'cli' (bootstrap).
CREATE TABLE audit_log (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id     uuid NOT NULL REFERENCES tenants (id),
    source        text NOT NULL CHECK (source IN ('api', 'cli')),
    actor_user_id uuid,
    token_id      uuid,
    request_id    text NOT NULL DEFAULT '',
    action        text NOT NULL,
    entity        text NOT NULL,
    entity_id     uuid NOT NULL,
    summary       jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_tenant_idx ON audit_log (tenant_id, id);

-- ---------------------------------------------------------------------------
-- Row-level security: a row is visible only when it belongs to the transaction's tenant.
-- current_setting(..., true) is NULL when unset, so with no tenant no row matches (fail closed).
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['memberships', 'role_assignments', 'api_tokens', 'tenant_counters',
                             'families', 'guardians', 'students', 'guardian_links', 'audit_log']
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %I
               USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid)
               WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- Append-only enforcement, for every role including the owner.
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
CREATE FUNCTION forbid_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only (DEC-011)', TG_TABLE_NAME;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER audit_log_append_only BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- ---------------------------------------------------------------------------
-- Controlled cross-tenant reads (ADR-0011 §3). Each returns the minimum needed.
-- They run as the owner (SECURITY DEFINER) and are executable by the API role only.
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
CREATE FUNCTION resolve_api_token(p_hash bytea)
RETURNS TABLE (token_id uuid, tenant_id uuid, membership_id uuid, user_id uuid,
               kind text, expires_at timestamptz, revoked_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT t.id, t.tenant_id, t.membership_id, t.user_id, t.kind, t.expires_at, t.revoked_at
    FROM api_tokens t
    WHERE t.token_hash = p_hash
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION tenant_id_by_slug(p_slug text)
RETURNS TABLE (id uuid, slug text, name text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT t.id, t.slug, t.name FROM tenants t WHERE t.slug = p_slug
$$;
-- +goose StatementEnd

REVOKE EXECUTE ON FUNCTION resolve_api_token(bytea) FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION tenant_id_by_slug(text) FROM PUBLIC;

-- ---------------------------------------------------------------------------
-- Grants for the API role. Nothing on tenants or users is writable by it.
-- ---------------------------------------------------------------------------
GRANT USAGE ON SCHEMA public TO app_rt;
GRANT SELECT ON users TO app_rt;
GRANT SELECT, INSERT, UPDATE ON otp_challenges TO app_rt;
GRANT SELECT, INSERT, UPDATE, DELETE ON memberships, role_assignments, api_tokens,
    tenant_counters, families, guardians, students, guardian_links TO app_rt;
GRANT SELECT, INSERT ON audit_log TO app_rt;
GRANT USAGE ON SEQUENCE audit_log_id_seq TO app_rt;
GRANT EXECUTE ON FUNCTION resolve_api_token(bytea), tenant_id_by_slug(text) TO app_rt;

-- +goose Down
DROP FUNCTION IF EXISTS tenant_id_by_slug(text);
DROP FUNCTION IF EXISTS resolve_api_token(bytea);
DROP TRIGGER IF EXISTS audit_log_append_only ON audit_log;
DROP FUNCTION IF EXISTS forbid_mutation();
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS guardian_links;
DROP TABLE IF EXISTS students;
DROP TABLE IF EXISTS guardians;
DROP TABLE IF EXISTS families;
DROP TABLE IF EXISTS tenant_counters;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS role_assignments;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS otp_challenges;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
