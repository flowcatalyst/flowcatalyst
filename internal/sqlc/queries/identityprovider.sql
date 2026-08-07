-- Queries for oauth_identity_providers + oauth_identity_provider_allowed_roles.
-- The allowed-roles junction (one row per platform role the IDP may confer via
-- role sync) lives here since migration 040. Allowed email domains are NOT
-- stored on the provider — they are derived from tnt_email_domain_mappings
-- (see IdentityProviderMappedDomainsForIDPs), which is the single source of
-- truth for domain → IDP routing. The legacy
-- oauth_identity_provider_allowed_domains junction is dead.

-- name: IdentityProviderFindByID :one
SELECT id, code, name, type, oidc_issuer_url, oidc_client_id,
       oidc_client_secret_ref, oidc_multi_tenant, oidc_issuer_pattern,
       created_at, updated_at, sync_roles_from_idp
FROM oauth_identity_providers
WHERE id = $1;

-- name: IdentityProviderFindByCode :one
SELECT id, code, name, type, oidc_issuer_url, oidc_client_id,
       oidc_client_secret_ref, oidc_multi_tenant, oidc_issuer_pattern,
       created_at, updated_at, sync_roles_from_idp
FROM oauth_identity_providers
WHERE code = $1;

-- name: IdentityProviderFindAll :many
SELECT id, code, name, type, oidc_issuer_url, oidc_client_id,
       oidc_client_secret_ref, oidc_multi_tenant, oidc_issuer_pattern,
       created_at, updated_at, sync_roles_from_idp
FROM oauth_identity_providers
ORDER BY code;

-- name: IdentityProviderUpsert :exec
INSERT INTO oauth_identity_providers
    (id, code, name, type, oidc_issuer_url, oidc_client_id,
     oidc_client_secret_ref, oidc_multi_tenant, oidc_issuer_pattern,
     sync_roles_from_idp, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (id) DO UPDATE SET
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    oidc_issuer_url = EXCLUDED.oidc_issuer_url,
    oidc_client_id = EXCLUDED.oidc_client_id,
    oidc_client_secret_ref = EXCLUDED.oidc_client_secret_ref,
    oidc_multi_tenant = EXCLUDED.oidc_multi_tenant,
    oidc_issuer_pattern = EXCLUDED.oidc_issuer_pattern,
    sync_roles_from_idp = EXCLUDED.sync_roles_from_idp,
    updated_at = EXCLUDED.updated_at;

-- name: IdentityProviderDelete :exec
DELETE FROM oauth_identity_providers WHERE id = $1;

-- name: IdentityProviderDomainsClear :exec
DELETE FROM oauth_identity_provider_allowed_domains WHERE identity_provider_id = $1;

-- name: IdentityProviderAllowedRolesClear :exec
DELETE FROM oauth_identity_provider_allowed_roles WHERE identity_provider_id = $1;

-- name: IdentityProviderAllowedRoleInsert :exec
INSERT INTO oauth_identity_provider_allowed_roles
    (identity_provider_id, role_id)
VALUES (@identity_provider_id, @role_id);

-- name: IdentityProviderAllowedRolesForIDPs :many
SELECT identity_provider_id, role_id
FROM oauth_identity_provider_allowed_roles
WHERE identity_provider_id = ANY(@idp_ids::text[])
ORDER BY identity_provider_id, role_id;

-- name: IdentityProviderMappedDomainsForIDPs :many
SELECT identity_provider_id, email_domain
FROM tnt_email_domain_mappings
WHERE identity_provider_id = ANY(@idp_ids::text[])
ORDER BY identity_provider_id, email_domain;
