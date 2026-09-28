-- Queries for fng_functions, fng_versions, fng_aliases, fng_settings,
-- fng_runners, fng_pool_revisions.

-- name: FunctionFindByID :one
SELECT sqlc.embed(f), a.code AS application_code
FROM fng_functions f
LEFT JOIN app_applications a ON a.id = f.application_id
WHERE f.id = $1;

-- name: FunctionFindByAddress :one
SELECT sqlc.embed(f), a.code AS application_code
FROM fng_functions f
LEFT JOIN app_applications a ON a.id = f.application_id
WHERE f.address = $1;

-- name: FunctionUpsert :exec
INSERT INTO fng_functions
    (id, application_id, client_id, name, address, description, pool, warm,
     limits, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (id) DO UPDATE SET
    description = EXCLUDED.description,
    pool = EXCLUDED.pool,
    warm = EXCLUDED.warm,
    limits = EXCLUDED.limits,
    updated_at = EXCLUDED.updated_at;

-- name: FunctionDelete :exec
DELETE FROM fng_functions WHERE id = $1;

-- name: FunctionVersionInsert :exec
INSERT INTO fng_versions
    (id, function_id, number, digest, size_bytes, abi, describe, status,
     failure, ready_at, published_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: FunctionVersionListByFunction :many
SELECT id, function_id, number, digest, size_bytes, abi, describe, status,
       failure, ready_at, published_by, created_at
FROM fng_versions
WHERE function_id = $1
ORDER BY number DESC;

-- name: FunctionVersionGetByNumber :one
SELECT id, function_id, number, digest, size_bytes, abi, describe, status,
       failure, ready_at, published_by, created_at
FROM fng_versions
WHERE function_id = $1 AND number = $2;

-- name: FunctionVersionGetByDigest :one
SELECT id, function_id, number, digest, size_bytes, abi, describe, status,
       failure, ready_at, published_by, created_at
FROM fng_versions
WHERE function_id = $1 AND digest = $2;

-- name: FunctionVersionSetStatus :exec
UPDATE fng_versions SET status = $2 WHERE id = $1;

-- name: FunctionVersionSetReady :exec
UPDATE fng_versions SET status = 'READY', ready_at = $2 WHERE id = $1;

-- name: FunctionVersionSetFailure :exec
UPDATE fng_versions SET status = 'FAILED', failure = $2 WHERE id = $1;

-- name: FunctionAliasUpsert :exec
INSERT INTO fng_aliases (function_id, name, version_id, updated_at, updated_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (function_id, name) DO UPDATE SET
    version_id = EXCLUDED.version_id,
    updated_at = EXCLUDED.updated_at,
    updated_by = EXCLUDED.updated_by;

-- name: FunctionAliasDelete :exec
DELETE FROM fng_aliases WHERE function_id = $1 AND name = $2;

-- name: FunctionAliasList :many
SELECT function_id, name, version_id, updated_at, updated_by
FROM fng_aliases
WHERE function_id = $1
ORDER BY name;

-- name: FunctionSettingUpsert :exec
INSERT INTO fng_settings (function_id, kind, key, value, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (function_id, kind, key) DO UPDATE SET
    value = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at;

-- name: FunctionSettingDelete :exec
DELETE FROM fng_settings WHERE function_id = $1 AND kind = $2 AND key = $3;

-- name: FunctionSettingList :many
SELECT function_id, kind, key, value, updated_at
FROM fng_settings
WHERE function_id = $1
ORDER BY kind, key;

-- name: FunctionRunnerUpsertHeartbeat :exec
INSERT INTO fng_runners (id, pool, heartbeat_at, report)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET
    pool = EXCLUDED.pool,
    heartbeat_at = EXCLUDED.heartbeat_at,
    report = EXCLUDED.report;

-- name: FunctionPoolRevisionBump :one
INSERT INTO fng_pool_revisions (pool, revision)
VALUES ($1, 1)
ON CONFLICT (pool) DO UPDATE SET revision = fng_pool_revisions.revision + 1
RETURNING revision;

-- name: FunctionPoolRevisionGet :one
SELECT revision FROM fng_pool_revisions WHERE pool = $1;
