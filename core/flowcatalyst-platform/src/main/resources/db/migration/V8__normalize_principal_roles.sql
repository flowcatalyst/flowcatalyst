-- V8: Normalize principal_roles from JSONB to proper table
--
-- The principals.roles column stores role assignments as JSONB:
-- [{"roleName": "platform:admin", "assignmentSource": "MANUAL", "assignedAt": "2024-..."}]
--
-- This migration creates a proper junction table for better querying and referential integrity.

-- =============================================================================
-- Create principal_roles table
-- =============================================================================

CREATE TABLE principal_roles (
    principal_id VARCHAR(17) NOT NULL,
    role_name VARCHAR(100) NOT NULL,
    assignment_source VARCHAR(50),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (principal_id, role_name)
);

-- Index for finding all principals with a specific role
CREATE INDEX idx_principal_roles_role_name ON principal_roles(role_name);

-- Index for finding when roles were assigned (useful for auditing)
CREATE INDEX idx_principal_roles_assigned_at ON principal_roles(assigned_at);

-- =============================================================================
-- Migrate existing JSONB data
-- =============================================================================

-- Insert rows from the existing JSONB column
-- Handle assignedAt as either epoch milliseconds (numeric) or ISO timestamp string
INSERT INTO principal_roles (principal_id, role_name, assignment_source, assigned_at)
SELECT
    p.id as principal_id,
    role_elem->>'roleName' as role_name,
    role_elem->>'assignmentSource' as assignment_source,
    COALESCE(
        CASE
            -- If it's a numeric value (epoch milliseconds), convert using to_timestamp
            WHEN role_elem->>'assignedAt' ~ '^\d+\.?\d*$'
            THEN to_timestamp((role_elem->>'assignedAt')::double precision / 1000)
            -- Otherwise try to parse as ISO timestamp
            ELSE (role_elem->>'assignedAt')::timestamptz
        END,
        p.created_at
    ) as assigned_at
FROM principals p,
     jsonb_array_elements(COALESCE(p.roles::jsonb, '[]'::jsonb)) as role_elem
WHERE p.roles IS NOT NULL
  AND p.roles != '[]'
  AND role_elem->>'roleName' IS NOT NULL;

-- =============================================================================
-- Verification query (run manually to check migration)
-- =============================================================================
-- SELECT p.id,
--        jsonb_array_length(COALESCE(p.roles::jsonb, '[]')) as jsonb_count,
--        (SELECT COUNT(*) FROM principal_roles pr WHERE pr.principal_id = p.id) as table_count
-- FROM principals p
-- WHERE jsonb_array_length(COALESCE(p.roles::jsonb, '[]')) !=
--       (SELECT COUNT(*) FROM principal_roles pr WHERE pr.principal_id = p.id);
-- Should return 0 rows if migration was successful

-- =============================================================================
-- NOTE: The principals.roles column is NOT dropped yet.
-- It will be removed in a future migration (V12) after:
-- 1. Application code is updated to use the new table
-- 2. Validation period confirms data integrity
-- =============================================================================
