-- V19: Add managed application scope for principals
--
-- This migration adds support for controlling which applications a principal can manage.
-- A principal with SPECIFIC scope can only create roles, permissions, event types, etc.
-- for the applications listed in the junction table.
--
-- This is used for:
-- - Application service accounts (automatically scoped to their application)
-- - Application admins (users granted access to manage specific applications)
-- - Platform admins (ALL scope - can manage all applications)

-- =============================================================================
-- Add managed_application_scope column to principals
-- =============================================================================

ALTER TABLE principals
ADD COLUMN managed_application_scope VARCHAR(20) DEFAULT 'NONE';

-- Add index for efficient filtering
CREATE INDEX idx_principals_managed_scope ON principals(managed_application_scope);

-- =============================================================================
-- Create principal_managed_applications junction table
-- =============================================================================

CREATE TABLE principal_managed_applications (
    principal_id VARCHAR(17) NOT NULL,
    application_id VARCHAR(17) NOT NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (principal_id, application_id)
);

-- Index for finding all principals that can manage an application
CREATE INDEX idx_principal_managed_apps_app_id ON principal_managed_applications(application_id);

-- Index for finding when access was granted (useful for auditing)
CREATE INDEX idx_principal_managed_apps_granted_at ON principal_managed_applications(granted_at);

-- =============================================================================
-- Migrate existing application_id data
-- =============================================================================

-- For service accounts that have an application_id set, migrate them to:
-- 1. Set managed_application_scope = 'SPECIFIC'
-- 2. Add a row to principal_managed_applications

-- Update scope for principals with application_id
UPDATE principals
SET managed_application_scope = 'SPECIFIC'
WHERE application_id IS NOT NULL;

-- Insert rows into junction table for principals with application_id
INSERT INTO principal_managed_applications (principal_id, application_id, granted_at)
SELECT id, application_id, created_at
FROM principals
WHERE application_id IS NOT NULL;

-- =============================================================================
-- Migration for platform admins
-- =============================================================================

-- Set ALL scope for principals with platform admin roles
UPDATE principals p
SET managed_application_scope = 'ALL'
WHERE EXISTS (
    SELECT 1 FROM principal_roles pr
    WHERE pr.principal_id = p.id
    AND pr.role_name IN ('platform:super-admin', 'platform:platform-admin')
);

-- =============================================================================
-- Verification query (run manually to check migration)
-- =============================================================================
-- SELECT
--     p.id,
--     p.name,
--     p.type,
--     p.application_id,
--     p.managed_application_scope,
--     (SELECT COUNT(*) FROM principal_managed_applications pma WHERE pma.principal_id = p.id) as managed_app_count
-- FROM principals p
-- WHERE p.application_id IS NOT NULL
--    OR p.managed_application_scope != 'NONE'
-- ORDER BY p.managed_application_scope, p.type;

-- =============================================================================
-- NOTE: The application_id column is NOT dropped yet.
-- It will be removed in a future migration after:
-- 1. Application code is fully migrated to use managedApplicationIds
-- 2. Validation period confirms data integrity
-- =============================================================================
