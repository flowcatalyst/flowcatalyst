-- V30: Replace managed application scope with direct application access
--
-- This migration replaces the managed_application_scope model with a simpler
-- direct user-to-application access model. Users get no applications by default
-- and must be explicitly granted access to each application.
--
-- Key changes:
-- 1. Create new principal_application_access junction table
-- 2. Migrate existing data from principal_managed_applications
-- 3. Remove managed_application_scope column
-- 4. Drop principal_managed_applications table

-- =============================================================================
-- Create new junction table for user-to-application access
-- =============================================================================

CREATE TABLE principal_application_access (
    principal_id VARCHAR(17) NOT NULL,
    application_id VARCHAR(17) NOT NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (principal_id, application_id)
);

-- Index for finding all principals with access to a specific application
CREATE INDEX idx_principal_app_access_app_id ON principal_application_access(application_id);

-- =============================================================================
-- Migrate existing data
-- =============================================================================

-- Copy data from old managed applications table (if any exists)
INSERT INTO principal_application_access (principal_id, application_id, granted_at)
SELECT principal_id, application_id, granted_at
FROM principal_managed_applications
ON CONFLICT DO NOTHING;

-- =============================================================================
-- Remove old managed application scope model
-- =============================================================================

-- Drop the scope column from principals table
ALTER TABLE principals DROP COLUMN IF EXISTS managed_application_scope;

-- Drop the index on the old column (if exists)
DROP INDEX IF EXISTS idx_principals_managed_scope;

-- Drop the old junction table
DROP TABLE IF EXISTS principal_managed_applications;
