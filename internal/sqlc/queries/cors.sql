-- Queries for tnt_cors_allowed_origins. Simple single-table CRUD.

-- name: CorsOriginFindByID :one
SELECT id, origin, description, created_by, created_at, updated_at
FROM tnt_cors_allowed_origins
WHERE id = $1;

-- name: CorsOriginFindByOrigin :one
SELECT id, origin, description, created_by, created_at, updated_at
FROM tnt_cors_allowed_origins
WHERE origin = $1;

-- name: CorsOriginFindAll :many
SELECT id, origin, description, created_by, created_at, updated_at
FROM tnt_cors_allowed_origins
ORDER BY origin;

-- name: CorsOriginListStrings :many
SELECT origin FROM tnt_cors_allowed_origins;

-- name: CorsOriginUpsert :exec
INSERT INTO tnt_cors_allowed_origins
    (id, origin, description, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    origin = EXCLUDED.origin,
    description = EXCLUDED.description,
    updated_at = EXCLUDED.updated_at;

-- name: CorsOriginDelete :exec
DELETE FROM tnt_cors_allowed_origins WHERE id = $1;
