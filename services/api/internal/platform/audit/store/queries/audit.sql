-- name: InsertAuditEntry :exec
INSERT INTO audit_log (tenant_id, source, actor_user_id, token_id, request_id,
                       action, entity, entity_id, summary)
VALUES (sqlc.arg('tenant_id'), sqlc.arg('source'), sqlc.narg('actor_user_id'), sqlc.narg('token_id'),
        sqlc.arg('request_id'), sqlc.arg('action'), sqlc.arg('entity'), sqlc.arg('entity_id'),
        sqlc.arg('summary'));
