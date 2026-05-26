-- Queries for oauth_identity_providers + oauth_identity_provider_allowed_domains.
-- Email domains are stored in the junction table (one row per allowed domain),
-- not as a column on the parent — the previous Go port incorrectly treated
-- this as a single JSONB-style column.

-- name: IdentityProviderFindByID :one
SELECT id, code, name, type, oidc_issuer_url, oidc_client_id,
       oidc_client_secret_ref, oidc_multi_tenant, oidc_issuer_pattern,
       created_at, updated_at
FROM oauth_identity_providers
WHERE id = $1;

-- name: IdentityProviderFindByCode :one
SELECT id, code, name, type, oidc_issuer_url, oidc_client_id,
       oidc_client_secret_ref, oidc_multi_tenant, oidc_issuer_pattern,
       created_at, updated_at
FROM oauth_identity_providers
WHERE code = $1;

-- name: IdentityProviderFindAll :many
SELECT id, code, name, type, oidc_issuer_url, oidc_client_id,
       oidc_client_secret_ref, oidc_multi_tenant, oidc_issuer_pattern,
       created_at, updated_at
FROM oauth_identity_providers
ORDER BY code;

-- name: IdentityProviderUpsert :exec
INSERT INTO oauth_identity_providers
    (id, code, name, type, oidc_issuer_url, oidc_client_id,
     oidc_client_secret_ref, oidc_multi_tenant, oidc_issuer_pattern,
     created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO UPDATE SET
    code = EXCLUDED.code,
    name = EXCLUDED.name,
    oidc_issuer_url = EXCLUDED.oidc_issuer_url,
    oidc_client_id = EXCLUDED.oidc_client_id,
    oidc_client_secret_ref = EXCLUDED.oidc_client_secret_ref,
    oidc_multi_tenant = EXCLUDED.oidc_multi_tenant,
    oidc_issuer_pattern = EXCLUDED.oidc_issuer_pattern,
    updated_at = EXCLUDED.updated_at;

-- name: IdentityProviderDelete :exec
DELETE FROM oauth_identity_providers WHERE id = $1;

-- name: IdentityProviderDomainsClear :exec
DELETE FROM oauth_identity_provider_allowed_domains WHERE identity_provider_id = $1;

-- name: IdentityProviderDomainInsert :exec
INSERT INTO oauth_identity_provider_allowed_domains
    (identity_provider_id, email_domain)
VALUES (@identity_provider_id, @email_domain);

-- name: IdentityProviderDomainsForIDPs :many
SELECT identity_provider_id, email_domain
FROM oauth_identity_provider_allowed_domains
WHERE identity_provider_id = ANY(@idp_ids::text[])
ORDER BY identity_provider_id, email_domain;
