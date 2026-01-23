-- V27: Drop service_account_roles table
--
-- Service account roles are now stored on the Principal entity via the principal_roles table.
-- This consolidates role storage to a single location (Principal) for both users and service accounts.
--
-- The service_account_roles table was created in V9 and is no longer used.
-- All role assignment operations now go through Principal.roles -> principal_roles table.

-- =============================================================================
-- Migrate any remaining data to principal_roles (just in case)
-- =============================================================================

-- First, insert any service account roles that don't already exist in principal_roles
INSERT INTO principal_roles (principal_id, role_name, assignment_source, assigned_at)
SELECT
    p.id as principal_id,
    sar.role_name,
    COALESCE(sar.assignment_source, 'admin') as assignment_source,
    COALESCE(sar.assigned_at, NOW()) as assigned_at
FROM service_account_roles sar
JOIN principals p ON p.service_account_id = sar.service_account_id
WHERE NOT EXISTS (
    SELECT 1 FROM principal_roles pr
    WHERE pr.principal_id = p.id AND pr.role_name = sar.role_name
);

-- =============================================================================
-- Drop the service_account_roles table
-- =============================================================================

DROP TABLE IF EXISTS service_account_roles;
