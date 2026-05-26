-- Queries for app_platform_configs + app_platform_config_access. Two
-- entity types, one repo (mirroring the Rust shape).

-- name: PlatformConfigFindByID :one
SELECT id, application_code, section, property, scope, client_id,
       value_type, value, description, created_at, updated_at
FROM app_platform_configs
WHERE id = $1;

-- name: PlatformConfigFindByCoordinateClient :one
SELECT id, application_code, section, property, scope, client_id,
       value_type, value, description, created_at, updated_at
FROM app_platform_configs
WHERE application_code = $1 AND section = $2 AND property = $3
  AND scope = $4 AND client_id = $5;

-- name: PlatformConfigFindByCoordinateAnchor :one
SELECT id, application_code, section, property, scope, client_id,
       value_type, value, description, created_at, updated_at
FROM app_platform_configs
WHERE application_code = $1 AND section = $2 AND property = $3
  AND scope = $4 AND client_id IS NULL;

-- name: PlatformConfigFindByApplication :many
SELECT id, application_code, section, property, scope, client_id,
       value_type, value, description, created_at, updated_at
FROM app_platform_configs
WHERE application_code = $1
ORDER BY section, property;

-- name: PlatformConfigUpsert :exec
INSERT INTO app_platform_configs
    (id, application_code, section, property, scope, client_id,
     value_type, value, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO UPDATE SET
    value_type = EXCLUDED.value_type,
    value = EXCLUDED.value,
    description = EXCLUDED.description,
    updated_at = EXCLUDED.updated_at;

-- name: PlatformConfigDelete :exec
DELETE FROM app_platform_configs WHERE id = $1;

-- name: PlatformConfigAccessFindByID :one
SELECT id, application_code, role_code, can_read, can_write, created_at
FROM app_platform_config_access
WHERE id = $1;

-- name: PlatformConfigAccessFindByRole :one
SELECT id, application_code, role_code, can_read, can_write, created_at
FROM app_platform_config_access
WHERE application_code = $1 AND role_code = $2;

-- name: PlatformConfigAccessFindByApplication :many
SELECT id, application_code, role_code, can_read, can_write, created_at
FROM app_platform_config_access
WHERE application_code = $1
ORDER BY role_code;

-- name: PlatformConfigAccessUpsert :exec
INSERT INTO app_platform_config_access
    (id, application_code, role_code, can_read, can_write, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    can_read = EXCLUDED.can_read,
    can_write = EXCLUDED.can_write;

-- name: PlatformConfigAccessDelete :exec
DELETE FROM app_platform_config_access WHERE id = $1;

-- name: PlatformConfigAccessHasReadByRoles :one
SELECT EXISTS(
    SELECT 1 FROM app_platform_config_access
    WHERE application_code = $1
      AND role_code = ANY(@role_codes::text[])
      AND can_read = true
);

-- name: PlatformConfigAccessHasWriteByRoles :one
SELECT EXISTS(
    SELECT 1 FROM app_platform_config_access
    WHERE application_code = $1
      AND role_code = ANY(@role_codes::text[])
      AND can_write = true
);
