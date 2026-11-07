-- +goose Up
-- Academic structure and enrolments (org module + students enrolment history).
-- Hierarchy for now: tenant > branch > (session, class > section). The institution level is added
-- later by a backward-compatible migration (ADR-0013). A tenant's default branch is created the
-- first time the org module needs it, so existing tenants need no backfill.
-- No table here grants DELETE to the API role: business records are archived, never deleted (DEC-011).

CREATE TABLE branches (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    name       text NOT NULL,
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name)
);
-- At most one default branch per tenant; lets "create it if missing" be a single safe statement.
CREATE UNIQUE INDEX branches_one_default ON branches (tenant_id) WHERE is_default;

-- An academic year. BR-ORG-001: a branch has exactly one current session at a time.
CREATE TABLE sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    branch_id  uuid NOT NULL,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
    starts_on  date NOT NULL,
    ends_on    date NOT NULL,
    is_current boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    CHECK (ends_on > starts_on),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, branch_id, name),
    FOREIGN KEY (tenant_id, branch_id) REFERENCES branches (tenant_id, id)
);
CREATE UNIQUE INDEX sessions_one_current ON sessions (tenant_id, branch_id) WHERE is_current;

CREATE TABLE classes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants (id),
    branch_id   uuid NOT NULL,
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
    grade_level integer NOT NULL CHECK (grade_level BETWEEN 0 AND 16),
    status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    created_by  uuid,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, branch_id, name),
    FOREIGN KEY (tenant_id, branch_id) REFERENCES branches (tenant_id, id)
);

-- REQ-ORG-005: the shift is a property of the section. 5-A Morning and 5-A Evening are different rows.
CREATE TABLE sections (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    class_id   uuid NOT NULL,
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 20),
    shift      text NOT NULL CHECK (shift IN ('morning', 'evening')),
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, class_id, name, shift),
    FOREIGN KEY (tenant_id, class_id) REFERENCES classes (tenant_id, id)
);

-- Enrolment history (REQ-STU-008): one row per spell in a section. A move closes the old row
-- and opens a new one; nothing is overwritten. BR-ORG-003: a section with enrolments cannot be
-- deleted, because these foreign keys have no ON DELETE action and the API role has no DELETE.
CREATE TABLE enrolments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    student_id uuid NOT NULL,
    session_id uuid NOT NULL,
    section_id uuid NOT NULL,
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    started_on date NOT NULL,
    ended_on   date,
    reason     text,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by uuid,
    CHECK ((status = 'active' AND ended_on IS NULL) OR (status = 'closed' AND ended_on IS NOT NULL)),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, student_id) REFERENCES students (tenant_id, id),
    FOREIGN KEY (tenant_id, session_id) REFERENCES sessions (tenant_id, id),
    FOREIGN KEY (tenant_id, section_id) REFERENCES sections (tenant_id, id)
);
-- A student has at most one active enrolment per session.
CREATE UNIQUE INDEX enrolments_one_active_per_session ON enrolments (tenant_id, student_id, session_id) WHERE status = 'active';
CREATE INDEX enrolments_student_idx ON enrolments (tenant_id, student_id, created_at DESC, id DESC);
CREATE INDEX enrolments_section_idx ON enrolments (tenant_id, section_id) WHERE status = 'active';

-- Row-level security, with the NULLIF form fixed in 00003.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['branches', 'sessions', 'classes', 'sections', 'enrolments']
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

-- Grants: no DELETE anywhere in this migration (DEC-011).
GRANT SELECT, INSERT, UPDATE ON branches, sessions, classes, sections, enrolments TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS enrolments;
DROP TABLE IF EXISTS sections;
DROP TABLE IF EXISTS classes;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS branches;
