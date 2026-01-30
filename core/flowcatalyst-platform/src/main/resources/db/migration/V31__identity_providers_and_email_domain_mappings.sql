-- V31: Create identity_providers and email_domain_mappings tables
-- Splits the existing ClientAuthConfig model into two entities with 1:N relationship

-- Identity Providers table
CREATE TABLE identity_providers (
    id VARCHAR(17) PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(200) NOT NULL,
    type VARCHAR(20) NOT NULL,
    oidc_issuer_url VARCHAR(500),
    oidc_client_id VARCHAR(200),
    oidc_client_secret_ref VARCHAR(500),
    oidc_multi_tenant BOOLEAN DEFAULT FALSE,
    oidc_issuer_pattern VARCHAR(500),
    allowed_email_domains TEXT[],
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- Email Domain Mappings table
CREATE TABLE email_domain_mappings (
    id VARCHAR(17) PRIMARY KEY,
    email_domain VARCHAR(255) NOT NULL UNIQUE,
    identity_provider_id VARCHAR(17) NOT NULL,
    scope_type VARCHAR(20) NOT NULL,
    primary_client_id VARCHAR(17),
    additional_client_ids TEXT[],
    granted_client_ids TEXT[],
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- Index for faster lookups by identity provider
CREATE INDEX idx_edm_idp ON email_domain_mappings(identity_provider_id);

-- Index for lookup by email domain (common query pattern)
CREATE INDEX idx_edm_email_domain ON email_domain_mappings(email_domain);

-- Index for lookup by scope type
CREATE INDEX idx_edm_scope_type ON email_domain_mappings(scope_type);

-- Index for IDP lookup by code
CREATE INDEX idx_idp_code ON identity_providers(code);

-- Index for IDP lookup by type
CREATE INDEX idx_idp_type ON identity_providers(type);
