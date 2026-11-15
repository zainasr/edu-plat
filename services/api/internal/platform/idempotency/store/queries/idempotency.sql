-- name: ClaimKey :one
-- Takes the key for one request. A new key inserts. A key whose earlier run failed, or that is older
-- than 24 hours, is taken over. Anything else is a live key: no row comes back and the caller reads it.
INSERT INTO idempotency_keys (tenant_id, key, endpoint, request_hash)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('key'), sqlc.arg('endpoint'), sqlc.arg('request_hash'))
ON CONFLICT (tenant_id, key) DO UPDATE
SET endpoint = EXCLUDED.endpoint, request_hash = EXCLUDED.request_hash, status = 'in_progress',
    owner = gen_random_uuid(), response_code = NULL, response_body = NULL, created_at = now(), completed_at = NULL
WHERE idempotency_keys.status = 'failed' OR idempotency_keys.created_at < now() - interval '24 hours'
RETURNING id, owner;

-- name: GetKey :one
SELECT endpoint, request_hash, status, response_code, response_body,
       (created_at < now() - interval '24 hours')::boolean AS expired
FROM idempotency_keys
WHERE tenant_id = sqlc.arg('tenant_id') AND key = sqlc.arg('key');

-- name: CompleteKey :execrows
-- Only the run that owns the key, while it is still in progress, may store its response.
UPDATE idempotency_keys
SET status = 'done', response_code = sqlc.arg('response_code'), response_body = sqlc.arg('response_body'),
    completed_at = now()
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND owner = sqlc.arg('owner') AND status = 'in_progress';

-- name: FailKey :execrows
-- The run did not succeed (an error or a panic): free the key so the client can retry and run again.
UPDATE idempotency_keys
SET status = 'failed', completed_at = now()
WHERE tenant_id = sqlc.arg('tenant_id') AND id = sqlc.arg('id') AND owner = sqlc.arg('owner') AND status = 'in_progress';
