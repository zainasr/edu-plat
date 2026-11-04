-- Runtime role for the API (ADR-0011 §1). Idempotent: safe to run on every start.
-- The API connects as app_rt. It is not the table owner, not a superuser and
-- has no BYPASSRLS, so row-level security applies to it.
-- Local development only: the password below is for the docker-compose database.
-- Staging and production create this role through infrastructure, not this file.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_rt') THEN
        CREATE ROLE app_rt LOGIN PASSWORD 'app_rt'
            NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
END
$$;
GRANT CONNECT ON DATABASE app_dev TO app_rt;
