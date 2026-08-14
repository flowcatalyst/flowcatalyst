-- Queries for iam_service_accounts. Webhook credentials are stored as
-- separate columns (wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref,
-- wh_signing_algorithm) matching the wire contract. The repository maps the flat
-- columns into a single WebhookCredentials struct in the aggregate.

-- name: ServiceAccountFindByID :one
SELECT id, code, name, description, application_id, active,
       wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref,
       wh_signing_algorithm, wh_credentials_created_at,
       wh_credentials_regenerated_at, last_used_at, created_at, updated_at,
       scope, client_ids
FROM iam_service_accounts
WHERE id = $1;

-- name: ServiceAccountFindByCode :one
SELECT id, code, name, description, application_id, active,
       wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref,
       wh_signing_algorithm, wh_credentials_created_at,
       wh_credentials_regenerated_at, last_used_at, created_at, updated_at,
       scope, client_ids
FROM iam_service_accounts
WHERE code = $1;

-- name: ServiceAccountFindFirstByApplicationID :one
-- The SA whose credentials sign the application's outbound deliveries.
-- Deterministic preference: the application's own provisioned service
-- account (app_applications.service_account_id → SERVICE principal →
-- iam_service_accounts) wins over any other SA that merely carries the
-- application_id; among the rest, oldest active first. Without this an app
-- with several linked SAs could sign with one SA while the operator rotates
-- credentials on another.
SELECT sa.id, sa.code, sa.name, sa.description, sa.application_id, sa.active,
       sa.wh_auth_type, sa.wh_auth_token_ref, sa.wh_signing_secret_ref,
       sa.wh_signing_algorithm, sa.wh_credentials_created_at,
       sa.wh_credentials_regenerated_at, sa.last_used_at, sa.created_at, sa.updated_at,
       sa.scope, sa.client_ids
FROM iam_service_accounts sa
WHERE sa.application_id = $1 AND sa.active = TRUE
ORDER BY
    (CASE WHEN EXISTS (
        SELECT 1
        FROM app_applications a
        JOIN iam_principals p ON p.id = a.service_account_id
        WHERE a.id = sa.application_id AND p.service_account_id = sa.id
    ) THEN 0 ELSE 1 END),
    sa.created_at ASC
LIMIT 1;

-- name: ServiceAccountFindAll :many
SELECT id, code, name, description, application_id, active,
       wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref,
       wh_signing_algorithm, wh_credentials_created_at,
       wh_credentials_regenerated_at, last_used_at, created_at, updated_at,
       scope, client_ids
FROM iam_service_accounts
ORDER BY code;

-- name: ServiceAccountUpsert :exec
INSERT INTO iam_service_accounts
    (id, code, name, description, application_id, scope, client_ids, active,
     wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref,
     wh_signing_algorithm, wh_credentials_created_at,
     wh_credentials_regenerated_at, last_used_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    application_id = EXCLUDED.application_id,
    scope = EXCLUDED.scope,
    client_ids = EXCLUDED.client_ids,
    active = EXCLUDED.active,
    wh_auth_type = EXCLUDED.wh_auth_type,
    wh_auth_token_ref = EXCLUDED.wh_auth_token_ref,
    wh_signing_secret_ref = EXCLUDED.wh_signing_secret_ref,
    wh_signing_algorithm = EXCLUDED.wh_signing_algorithm,
    wh_credentials_created_at = EXCLUDED.wh_credentials_created_at,
    wh_credentials_regenerated_at = EXCLUDED.wh_credentials_regenerated_at,
    last_used_at = EXCLUDED.last_used_at,
    updated_at = EXCLUDED.updated_at;

-- name: ServiceAccountDelete :exec
DELETE FROM iam_service_accounts WHERE id = $1;
