-- Queries for aud_logs (read-only — writes happen in platformsink).

-- name: AuditFindByID :one
SELECT a.id, a.entity_type, a.entity_id, a.operation, a.operation_json,
       a.principal_id, p.name AS principal_name,
       a.application_id, a.client_id, a.performed_at
FROM aud_logs a
LEFT JOIN iam_principals p ON p.id = a.principal_id
WHERE a.id = $1;

-- name: AuditFindWithFilters :many
-- All filters are optional via the IS-NULL-OR pattern. Limit + offset
-- are always bound. Ordered by most recent first.
SELECT a.id, a.entity_type, a.entity_id, a.operation, a.operation_json,
       a.principal_id, p.name AS principal_name,
       a.application_id, a.client_id, a.performed_at
FROM aud_logs a
LEFT JOIN iam_principals p ON p.id = a.principal_id
WHERE (sqlc.narg('entity_type')::text IS NULL OR a.entity_type = sqlc.narg('entity_type')::text)
  AND (sqlc.narg('entity_id')::text IS NULL OR a.entity_id = sqlc.narg('entity_id')::text)
  AND (sqlc.narg('principal_id')::text IS NULL OR a.principal_id = sqlc.narg('principal_id')::text)
  AND (sqlc.narg('client_id')::text IS NULL OR a.client_id = sqlc.narg('client_id')::text)
  AND (sqlc.narg('since')::timestamptz IS NULL OR a.performed_at >= sqlc.narg('since')::timestamptz)
  AND (sqlc.narg('until')::timestamptz IS NULL OR a.performed_at <= sqlc.narg('until')::timestamptz)
ORDER BY a.performed_at DESC
LIMIT sqlc.arg('lim')::int OFFSET sqlc.arg('off')::int;
