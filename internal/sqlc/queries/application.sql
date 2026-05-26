-- Queries for app_applications.

-- name: ApplicationFindByID :one
SELECT id, type, code, name, description, icon_url, website, logo, logo_mime_type,
       default_base_url, service_account_id, active, created_at, updated_at
FROM app_applications
WHERE id = $1;

-- name: ApplicationFindByCode :one
SELECT id, type, code, name, description, icon_url, website, logo, logo_mime_type,
       default_base_url, service_account_id, active, created_at, updated_at
FROM app_applications
WHERE code = $1;

-- name: ApplicationUpsert :exec
INSERT INTO app_applications
    (id, type, code, name, description, icon_url, website, logo, logo_mime_type,
     default_base_url, service_account_id, active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    type = EXCLUDED.type,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    icon_url = EXCLUDED.icon_url,
    website = EXCLUDED.website,
    logo = EXCLUDED.logo,
    logo_mime_type = EXCLUDED.logo_mime_type,
    default_base_url = EXCLUDED.default_base_url,
    service_account_id = EXCLUDED.service_account_id,
    active = EXCLUDED.active,
    updated_at = EXCLUDED.updated_at;

-- name: ApplicationDelete :exec
DELETE FROM app_applications WHERE id = $1;

-- name: ClientConfigFindByAppAndClient :one
SELECT id, application_id, client_id, enabled, created_at, updated_at
FROM app_client_configs
WHERE application_id = $1 AND client_id = $2;

-- name: ClientConfigFindByApp :many
SELECT id, application_id, client_id, enabled, created_at, updated_at
FROM app_client_configs
WHERE application_id = $1
ORDER BY created_at;

-- name: ClientConfigUpsert :exec
INSERT INTO app_client_configs
    (id, application_id, client_id, enabled, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    updated_at = EXCLUDED.updated_at;

-- name: ClientConfigDelete :exec
DELETE FROM app_client_configs WHERE id = $1;
