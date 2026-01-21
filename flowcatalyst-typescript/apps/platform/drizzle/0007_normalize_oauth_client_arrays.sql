-- V7: Normalize OAuth client array columns to collection tables
--
-- The oauth_clients table had JSONB array columns for redirect_uris, allowed_origins,
-- grant_types, and application_ids. This migration creates proper collection tables
-- for better querying and referential integrity.

-- =============================================================================
-- 1. Create Collection Tables
-- =============================================================================

-- Redirect URIs
CREATE TABLE oauth_client_redirect_uris (
    oauth_client_id VARCHAR(17) NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    redirect_uri VARCHAR(500) NOT NULL,
    PRIMARY KEY (oauth_client_id, redirect_uri)
);

CREATE INDEX idx_oauth_client_redirect_uris_client ON oauth_client_redirect_uris(oauth_client_id);

-- Allowed Origins (for CORS)
CREATE TABLE oauth_client_allowed_origins (
    oauth_client_id VARCHAR(17) NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    allowed_origin VARCHAR(200) NOT NULL,
    PRIMARY KEY (oauth_client_id, allowed_origin)
);

CREATE INDEX idx_oauth_client_allowed_origins_client ON oauth_client_allowed_origins(oauth_client_id);
CREATE INDEX idx_oauth_client_allowed_origins_origin ON oauth_client_allowed_origins(allowed_origin);

-- Grant Types
CREATE TABLE oauth_client_grant_types (
    oauth_client_id VARCHAR(17) NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    grant_type VARCHAR(50) NOT NULL,
    PRIMARY KEY (oauth_client_id, grant_type)
);

CREATE INDEX idx_oauth_client_grant_types_client ON oauth_client_grant_types(oauth_client_id);

-- Application IDs
CREATE TABLE oauth_client_application_ids (
    oauth_client_id VARCHAR(17) NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    application_id VARCHAR(17) NOT NULL,
    PRIMARY KEY (oauth_client_id, application_id)
);

CREATE INDEX idx_oauth_client_application_ids_client ON oauth_client_application_ids(oauth_client_id);

-- =============================================================================
-- 2. Migrate Data from JSONB Columns to Collection Tables (if columns exist)
-- =============================================================================

DO $$
BEGIN
    -- Check if redirect_uris column exists before migrating
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'oauth_clients' AND column_name = 'redirect_uris'
    ) THEN
        INSERT INTO oauth_client_redirect_uris (oauth_client_id, redirect_uri)
        SELECT id, jsonb_array_elements_text(redirect_uris)
        FROM oauth_clients
        WHERE redirect_uris IS NOT NULL AND jsonb_array_length(redirect_uris) > 0;
    END IF;

    -- Check if allowed_origins column exists before migrating
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'oauth_clients' AND column_name = 'allowed_origins'
    ) THEN
        INSERT INTO oauth_client_allowed_origins (oauth_client_id, allowed_origin)
        SELECT id, jsonb_array_elements_text(allowed_origins)
        FROM oauth_clients
        WHERE allowed_origins IS NOT NULL AND jsonb_array_length(allowed_origins) > 0;
    END IF;

    -- Check if grant_types column exists before migrating
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'oauth_clients' AND column_name = 'grant_types'
    ) THEN
        INSERT INTO oauth_client_grant_types (oauth_client_id, grant_type)
        SELECT id, jsonb_array_elements_text(grant_types)
        FROM oauth_clients
        WHERE grant_types IS NOT NULL AND jsonb_array_length(grant_types) > 0;
    END IF;

    -- Check if application_ids column exists before migrating
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'oauth_clients' AND column_name = 'application_ids'
    ) THEN
        INSERT INTO oauth_client_application_ids (oauth_client_id, application_id)
        SELECT id, jsonb_array_elements_text(application_ids)
        FROM oauth_clients
        WHERE application_ids IS NOT NULL AND jsonb_array_length(application_ids) > 0;
    END IF;
END $$;

-- =============================================================================
-- 3. Drop the old JSONB columns
-- =============================================================================

ALTER TABLE oauth_clients DROP COLUMN IF EXISTS redirect_uris;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS allowed_origins;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS grant_types;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS application_ids;
