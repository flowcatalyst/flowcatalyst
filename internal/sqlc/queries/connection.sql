-- Queries for msg_connections.

-- name: ConnectionFindByID :one
SELECT id, code, name, description, external_id, status, service_account_id,
       client_id, client_identifier, created_at, updated_at
FROM msg_connections
WHERE id = $1;

-- name: ConnectionFindByCodeClient :one
SELECT id, code, name, description, external_id, status, service_account_id,
       client_id, client_identifier, created_at, updated_at
FROM msg_connections
WHERE code = $1 AND client_id = $2;

-- name: ConnectionFindByCodeAnchor :one
SELECT id, code, name, description, external_id, status, service_account_id,
       client_id, client_identifier, created_at, updated_at
FROM msg_connections
WHERE code = $1 AND client_id IS NULL;

-- name: ConnectionFindAll :many
SELECT id, code, name, description, external_id, status, service_account_id,
       client_id, client_identifier, created_at, updated_at
FROM msg_connections
ORDER BY code;

-- name: ConnectionUpsert :exec
INSERT INTO msg_connections
    (id, code, name, description, external_id, status, service_account_id,
     client_id, client_identifier, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO UPDATE SET
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    external_id = EXCLUDED.external_id,
    status = EXCLUDED.status,
    service_account_id = EXCLUDED.service_account_id,
    client_id = EXCLUDED.client_id,
    client_identifier = EXCLUDED.client_identifier,
    updated_at = EXCLUDED.updated_at;

-- name: ConnectionDelete :exec
DELETE FROM msg_connections WHERE id = $1;
