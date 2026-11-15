-- +goose Up
-- Idempotency keys (slice 4a, ADR-0018, REQ-API-010). One row per (school, key): what the first request
-- was, whether it is still running, and the response to replay. The API role can insert and update but
-- never delete; rows older than 24 hours are treated as absent and re-claimed, and are tidied up by an
-- admin job later.

CREATE TABLE idempotency_keys (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants (id),
    key           text NOT NULL CHECK (key ~ '^[A-Za-z0-9._:-]{8,128}$'),
    endpoint      text NOT NULL,                       -- "POST /api/v1/challans/release"
    request_hash  bytea NOT NULL CHECK (length(request_hash) = 32),
    status        text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'done', 'failed')),
    -- Who holds the key right now. A takeover mints a new owner, so a run that lost the key cannot
    -- store its response over the new run's (it must present its owner to complete or fail the key).
    owner         uuid NOT NULL DEFAULT gen_random_uuid(),
    response_code integer CHECK (response_code BETWEEN 100 AND 599),
    response_body bytea,
    created_at    timestamptz NOT NULL DEFAULT now(),
    completed_at  timestamptz,
    CHECK (status <> 'done' OR (response_code IS NOT NULL AND response_body IS NOT NULL AND completed_at IS NOT NULL)),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, key)
);

-- Row-level security, with the NULLIF form from 00003.
ALTER TABLE idempotency_keys ENABLE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE 'CREATE POLICY tenant_isolation ON idempotency_keys
               USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)
               WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)';
END
$$;
-- +goose StatementEnd

GRANT SELECT, INSERT ON idempotency_keys TO app_rt;
GRANT UPDATE (endpoint, request_hash, status, owner, response_code, response_body, created_at, completed_at)
    ON idempotency_keys TO app_rt;

-- +goose Down
DROP TABLE IF EXISTS idempotency_keys;
