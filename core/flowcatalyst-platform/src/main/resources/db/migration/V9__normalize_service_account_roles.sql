-- V9: Normalize service_account_roles from JSONB to proper table
--
-- The service_accounts.roles column stores role assignments as JSONB:
-- [{"roleName": "platform:service-admin", "assignmentSource": "AUTO", "assignedAt": "2024-..."}]
--
-- This migration creates a proper junction table for better querying and referential integrity.

-- =============================================================================
-- Create service_account_roles table
-- =============================================================================

CREATE TABLE service_account_roles (
    service_account_id VARCHAR(17) NOT NULL,
    role_name VARCHAR(100) NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (service_account_id, role_name)
);

-- Index for finding all service accounts with a specific role
CREATE INDEX idx_service_account_roles_role_name ON service_account_roles(role_name);

-- =============================================================================
-- Migrate existing JSONB data
-- =============================================================================

-- Insert rows from the existing JSONB column
-- Handle assignedAt as either epoch milliseconds (numeric) or ISO timestamp string
INSERT INTO service_account_roles (service_account_id, role_name, assigned_at)
SELECT
    sa.id as service_account_id,
    role_elem->>'roleName' as role_name,
    COALESCE(
        CASE
            -- If it's a numeric value (epoch milliseconds), convert using to_timestamp
            WHEN role_elem->>'assignedAt' ~ '^\d+\.?\d*$'
            THEN to_timestamp((role_elem->>'assignedAt')::double precision / 1000)
            -- Otherwise try to parse as ISO timestamp
            ELSE (role_elem->>'assignedAt')::timestamptz
        END,
        sa.created_at
    ) as assigned_at
FROM service_accounts sa,
     jsonb_array_elements(COALESCE(sa.roles::jsonb, '[]'::jsonb)) as role_elem
WHERE sa.roles IS NOT NULL
  AND sa.roles != '[]'
  AND role_elem->>'roleName' IS NOT NULL;

-- =============================================================================
-- Verification query (run manually to check migration)
-- =============================================================================
-- SELECT sa.id,
--        jsonb_array_length(COALESCE(sa.roles::jsonb, '[]')) as jsonb_count,
--        (SELECT COUNT(*) FROM service_account_roles sar WHERE sar.service_account_id = sa.id) as table_count
-- FROM service_accounts sa
-- WHERE jsonb_array_length(COALESCE(sa.roles::jsonb, '[]')) !=
--       (SELECT COUNT(*) FROM service_account_roles sar WHERE sar.service_account_id = sa.id);
-- Should return 0 rows if migration was successful

-- =============================================================================
-- NOTE: The service_accounts.roles column is NOT dropped yet.
-- It will be removed in a future migration (V12) after:
-- 1. Application code is updated to use the new table
-- 2. Validation period confirms data integrity
-- =============================================================================
