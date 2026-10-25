-- Queries for app_platform_configs + app_platform_config_access. Two
-- entity types, one repo (mirroring the legacy-platform shape).

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

-- name: PlatformConfigListPlaintextSecrets :many
-- SECRET rows written before values were encrypted at rest (no "encrypted:"
-- marker). Drained by Repository.EncryptLegacySecrets at start-up.
SELECT id, application_code, section, property, client_id, value
FROM app_platform_configs
WHERE value_type = 'SECRET' AND value NOT LIKE 'encrypted:%';

-- name: PlatformConfigListEncryptedSecrets :many
-- SECRET rows stored encrypted, with the coordinate their ciphertext is bound
-- to. Repository.EncryptLegacySecrets re-encrypts the ones sealed before the
-- ciphertext carried its row's coordinate (no additional authenticated data).
SELECT id, application_code, section, property, client_id, value
FROM app_platform_configs
WHERE value_type = 'SECRET' AND value LIKE 'encrypted:%';

-- name: PlatformConfigCompareAndSetStoredValue :execrows
-- Replaces a SECRET's stored representation only while it is still the one
-- that was read (compare-and-set), so a conversion running on one replica
-- cannot undo a value another request set meanwhile. No updated_at bump: the
-- value did not change, only how it is stored.
UPDATE app_platform_configs
   SET value = sqlc.arg(new_value)
 WHERE id = sqlc.arg(id) AND value_type = 'SECRET' AND value = sqlc.arg(old_value);

-- name: PlatformConfigApplicationByCode :one
-- The application a config row would belong to (config rows are keyed by
-- application code, without a foreign key).
SELECT id, code FROM app_applications WHERE code = $1;

-- name: PlatformConfigServiceAccountClientIDs :one
-- The explicit client ids of the service account behind a principal. No row:
-- the principal is not a service account.
SELECT sa.client_ids
FROM iam_principals p
JOIN iam_service_accounts sa ON sa.id = p.service_account_id
WHERE p.id = $1;

-- name: PlatformConfigApplicationEnabledClientIDs :many
-- The clients an application is enabled for.
SELECT client_id FROM app_client_configs
WHERE application_id = $1 AND enabled
ORDER BY client_id;
