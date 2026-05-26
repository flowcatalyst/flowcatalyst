-- Queries for msg_dispatch_pools.

-- name: DispatchPoolFindByID :one
SELECT id, code, name, description, rate_limit, concurrency, client_id,
       client_identifier, status, created_at, updated_at
FROM msg_dispatch_pools
WHERE id = $1;

-- name: DispatchPoolFindByCodeClient :one
SELECT id, code, name, description, rate_limit, concurrency, client_id,
       client_identifier, status, created_at, updated_at
FROM msg_dispatch_pools
WHERE code = $1 AND client_id = $2;

-- name: DispatchPoolFindByCodeAnchor :one
SELECT id, code, name, description, rate_limit, concurrency, client_id,
       client_identifier, status, created_at, updated_at
FROM msg_dispatch_pools
WHERE code = $1 AND client_id IS NULL;

-- name: DispatchPoolFindAll :many
SELECT id, code, name, description, rate_limit, concurrency, client_id,
       client_identifier, status, created_at, updated_at
FROM msg_dispatch_pools
ORDER BY code;

-- name: DispatchPoolUpsert :exec
INSERT INTO msg_dispatch_pools
    (id, code, name, description, rate_limit, concurrency, client_id,
     client_identifier, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO UPDATE SET
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    rate_limit = EXCLUDED.rate_limit,
    concurrency = EXCLUDED.concurrency,
    client_id = EXCLUDED.client_id,
    client_identifier = EXCLUDED.client_identifier,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at;

-- name: DispatchPoolDelete :exec
DELETE FROM msg_dispatch_pools WHERE id = $1;
