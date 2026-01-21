-- V14: Normalize OAuth client array columns to collection tables
-- The JPA entity (OAuthClientEntity) uses @ElementCollection with @CollectionTable,
-- but the oauth_clients table was created with TEXT[] array columns.
-- This migration creates the expected collection tables and migrates the data.

-- =============================================================================
-- Create Collection Tables
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
-- Migrate Data from Array Columns to Collection Tables
-- =============================================================================

-- Migrate redirect_uris
INSERT INTO oauth_client_redirect_uris (oauth_client_id, redirect_uri)
SELECT id, unnest(redirect_uris)
FROM oauth_clients
WHERE redirect_uris IS NOT NULL AND array_length(redirect_uris, 1) > 0;

-- Migrate allowed_origins
INSERT INTO oauth_client_allowed_origins (oauth_client_id, allowed_origin)
SELECT id, unnest(allowed_origins)
FROM oauth_clients
WHERE allowed_origins IS NOT NULL AND array_length(allowed_origins, 1) > 0;

-- Migrate grant_types
INSERT INTO oauth_client_grant_types (oauth_client_id, grant_type)
SELECT id, unnest(grant_types)
FROM oauth_clients
WHERE grant_types IS NOT NULL AND array_length(grant_types, 1) > 0;

-- Migrate application_ids
INSERT INTO oauth_client_application_ids (oauth_client_id, application_id)
SELECT id, unnest(application_ids)
FROM oauth_clients
WHERE application_ids IS NOT NULL AND array_length(application_ids, 1) > 0;

-- =============================================================================
-- Drop the old array columns
-- =============================================================================

ALTER TABLE oauth_clients DROP COLUMN IF EXISTS redirect_uris;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS allowed_origins;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS grant_types;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS application_ids;
