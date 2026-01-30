-- V34: Add required_oidc_tenant_id to email_domain_mappings
--
-- For multi-tenant OIDC providers (like Azure AD/Entra), this field specifies
-- which tenant is authorized to authenticate users from this email domain.
-- This prevents users from unauthorized tenants from authenticating.

ALTER TABLE email_domain_mappings
ADD COLUMN IF NOT EXISTS required_oidc_tenant_id VARCHAR(100);

COMMENT ON COLUMN email_domain_mappings.required_oidc_tenant_id IS
    'Required OIDC tenant ID for multi-tenant IDPs. If set, the tid claim must match.';
