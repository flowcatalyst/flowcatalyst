-- All queries operating on tnt_clients. The Repository wrapper in
-- internal/platform/client maps the generated row type onto the
-- aggregate's Client struct.

-- name: ClientFindByID :one
SELECT id, name, identifier, status, status_reason, status_changed_at,
       notes, created_at, updated_at
FROM tnt_clients
WHERE id = $1;

-- name: ClientFindByIdentifier :one
SELECT id, name, identifier, status, status_reason, status_changed_at,
       notes, created_at, updated_at
FROM tnt_clients
WHERE identifier = $1;

-- name: ClientSearch :many
SELECT id, name, identifier, status, status_reason, status_changed_at,
       notes, created_at, updated_at
FROM tnt_clients
WHERE name ILIKE @pattern OR identifier ILIKE @pattern
ORDER BY identifier
LIMIT 50;

-- name: ClientFindAll :many
SELECT id, name, identifier, status, status_reason, status_changed_at,
       notes, created_at, updated_at
FROM tnt_clients
ORDER BY identifier;

-- name: ClientUpsert :exec
INSERT INTO tnt_clients
    (id, name, identifier, status, status_reason, status_changed_at,
     notes, created_at, updated_at)
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    identifier = EXCLUDED.identifier,
    status = EXCLUDED.status,
    status_reason = EXCLUDED.status_reason,
    status_changed_at = EXCLUDED.status_changed_at,
    notes = EXCLUDED.notes,
    updated_at = EXCLUDED.updated_at;

-- name: ClientDelete :exec
DELETE FROM tnt_clients WHERE id = $1;
