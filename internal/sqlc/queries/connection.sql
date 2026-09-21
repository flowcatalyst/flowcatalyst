-- Queries for msg_connections. Column lists are kept in the table's
-- physical column order (application_code/source were appended by migration
-- 056) so sqlc maps every query onto the shared MsgConnection model instead
-- of minting a one-off row type per query.

-- name: ConnectionFindByID :one
SELECT id, code, name, description, external_id, status, service_account_id,
       client_id, client_identifier, created_at, updated_at, application_code, source
FROM msg_connections
WHERE id = $1;

-- name: ConnectionFindByCode :one
-- NULL-as-a-value semantics on both nullable parts of the key: a caller
-- asking for "no application" (application_code = NULL) or "no client"
-- (client_id = NULL) must match rows stored with NULL there, which plain
-- `=` never does. Mirrors uq_msg_connections_app_client_code (migration 056).
SELECT id, code, name, description, external_id, status, service_account_id,
       client_id, client_identifier, created_at, updated_at, application_code, source
FROM msg_connections
WHERE code = sqlc.arg('code')
  AND application_code IS NOT DISTINCT FROM sqlc.narg('application_code')
  AND client_id IS NOT DISTINCT FROM sqlc.narg('client_id');

-- name: ConnectionFindAll :many
SELECT id, code, name, description, external_id, status, service_account_id,
       client_id, client_identifier, created_at, updated_at, application_code, source
FROM msg_connections
ORDER BY code;

-- name: ConnectionUpsert :exec
INSERT INTO msg_connections
    (id, code, name, description, external_id, status, service_account_id,
     client_id, client_identifier, created_at, updated_at, application_code, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (id) DO UPDATE SET
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    external_id = EXCLUDED.external_id,
    status = EXCLUDED.status,
    service_account_id = EXCLUDED.service_account_id,
    client_id = EXCLUDED.client_id,
    client_identifier = EXCLUDED.client_identifier,
    updated_at = EXCLUDED.updated_at,
    application_code = EXCLUDED.application_code,
    source = EXCLUDED.source;

-- name: ConnectionDelete :exec
DELETE FROM msg_connections WHERE id = $1;
