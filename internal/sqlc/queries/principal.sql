-- Queries for iam_principals + junction tables.
--
-- The schema stores user-identity fields as flat columns (email, email_domain,
-- idp_type, external_idp_id, password_hash, last_login_at) rather than the
-- JSONB blobs the Go entity carries. Mapping happens in repository.go.

-- name: PrincipalFindByID :one
SELECT id, type, scope, client_id, application_id, name, active,
       email, email_domain, idp_type, external_idp_id, password_hash,
       last_login_at, service_account_id, created_at, updated_at
FROM iam_principals
WHERE id = $1;

-- name: PrincipalFindByEmail :one
-- Case-insensitive match: emails are stored lower-cased (see repository.Persist),
-- but callers pass values from sources whose casing we don't control (OIDC
-- tokens, the login form). LOWER(email) also finds any legacy mixed-case row so
-- the login self-heal can normalise it. The repo lower-cases $1 before binding.
SELECT id, type, scope, client_id, application_id, name, active,
       email, email_domain, idp_type, external_idp_id, password_hash,
       last_login_at, service_account_id, created_at, updated_at
FROM iam_principals
WHERE type = 'USER' AND LOWER(email) = $1;

-- name: PrincipalFindAll :many
SELECT id, type, scope, client_id, application_id, name, active,
       email, email_domain, idp_type, external_idp_id, password_hash,
       last_login_at, service_account_id, created_at, updated_at
FROM iam_principals
ORDER BY created_at DESC;

-- name: PrincipalFindByServiceAccount :one
SELECT id, type, scope, client_id, application_id, name, active,
       email, email_domain, idp_type, external_idp_id, password_hash,
       last_login_at, service_account_id, created_at, updated_at
FROM iam_principals
WHERE type = 'SERVICE' AND service_account_id = $1;

-- name: PrincipalUpsert :exec
INSERT INTO iam_principals
    (id, type, scope, client_id, application_id, name, active,
     email, email_domain, idp_type, external_idp_id, password_hash,
     last_login_at, service_account_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
ON CONFLICT (id) DO UPDATE SET
    type = EXCLUDED.type,
    scope = EXCLUDED.scope,
    client_id = EXCLUDED.client_id,
    application_id = EXCLUDED.application_id,
    name = EXCLUDED.name,
    active = EXCLUDED.active,
    email = EXCLUDED.email,
    email_domain = EXCLUDED.email_domain,
    idp_type = EXCLUDED.idp_type,
    external_idp_id = EXCLUDED.external_idp_id,
    password_hash = EXCLUDED.password_hash,
    last_login_at = EXCLUDED.last_login_at,
    service_account_id = EXCLUDED.service_account_id,
    updated_at = EXCLUDED.updated_at;

-- name: PrincipalDelete :exec
DELETE FROM iam_principals WHERE id = $1;

-- iam_principal_application_access + iam_client_access_grants do NOT have
-- FK ON DELETE CASCADE on principal_id (only iam_principal_roles does), so
-- Delete has to clean them explicitly. Mirrors Rust's delete() + Persist::delete.

-- name: PrincipalApplicationAccessClear :exec
DELETE FROM iam_principal_application_access WHERE principal_id = $1;

-- name: PrincipalClientAccessGrantsClear :exec
DELETE FROM iam_client_access_grants WHERE principal_id = $1;
