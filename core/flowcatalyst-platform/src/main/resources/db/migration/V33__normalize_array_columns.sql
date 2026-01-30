-- V33: Normalize TEXT[] columns to proper junction tables
--
-- Converts three TEXT[] columns to @ElementCollection-compatible tables:
-- 1. email_domain_mappings.additional_client_ids -> email_domain_mapping_additional_clients
-- 2. email_domain_mappings.granted_client_ids -> email_domain_mapping_granted_clients
-- 3. identity_providers.allowed_email_domains -> identity_provider_allowed_domains

-- =============================================================================
-- 1. email_domain_mapping_additional_clients
-- =============================================================================

CREATE TABLE email_domain_mapping_additional_clients (
    email_domain_mapping_id VARCHAR(17) NOT NULL,
    client_id VARCHAR(17) NOT NULL,
    PRIMARY KEY (email_domain_mapping_id, client_id)
);

CREATE INDEX idx_edm_additional_clients_client ON email_domain_mapping_additional_clients(client_id);

-- Migrate existing data
INSERT INTO email_domain_mapping_additional_clients (email_domain_mapping_id, client_id)
SELECT id, unnest(additional_client_ids)
FROM email_domain_mappings
WHERE additional_client_ids IS NOT NULL AND array_length(additional_client_ids, 1) > 0;

-- =============================================================================
-- 2. email_domain_mapping_granted_clients
-- =============================================================================

CREATE TABLE email_domain_mapping_granted_clients (
    email_domain_mapping_id VARCHAR(17) NOT NULL,
    client_id VARCHAR(17) NOT NULL,
    PRIMARY KEY (email_domain_mapping_id, client_id)
);

CREATE INDEX idx_edm_granted_clients_client ON email_domain_mapping_granted_clients(client_id);

-- Migrate existing data
INSERT INTO email_domain_mapping_granted_clients (email_domain_mapping_id, client_id)
SELECT id, unnest(granted_client_ids)
FROM email_domain_mappings
WHERE granted_client_ids IS NOT NULL AND array_length(granted_client_ids, 1) > 0;

-- =============================================================================
-- 3. identity_provider_allowed_domains
-- =============================================================================

CREATE TABLE identity_provider_allowed_domains (
    identity_provider_id VARCHAR(17) NOT NULL,
    email_domain VARCHAR(255) NOT NULL,
    PRIMARY KEY (identity_provider_id, email_domain)
);

CREATE INDEX idx_idp_allowed_domains_domain ON identity_provider_allowed_domains(email_domain);

-- Migrate existing data
INSERT INTO identity_provider_allowed_domains (identity_provider_id, email_domain)
SELECT id, unnest(allowed_email_domains)
FROM identity_providers
WHERE allowed_email_domains IS NOT NULL AND array_length(allowed_email_domains, 1) > 0;

-- =============================================================================
-- 4. Drop the old TEXT[] columns
-- =============================================================================

ALTER TABLE email_domain_mappings DROP COLUMN IF EXISTS additional_client_ids;
ALTER TABLE email_domain_mappings DROP COLUMN IF EXISTS granted_client_ids;
ALTER TABLE identity_providers DROP COLUMN IF EXISTS allowed_email_domains;
