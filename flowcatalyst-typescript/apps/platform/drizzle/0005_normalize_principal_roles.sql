-- V5: Normalize principal_roles from JSONB to proper junction table
--
-- The principals.roles column stores role assignments as JSONB:
-- [{"roleName": "platform:admin", "assignmentSource": "MANUAL", "assignedAt": "2024-..."}]
--
-- This migration creates a proper junction table for better querying and referential integrity,
-- then drops the JSONB column.

-- =============================================================================
-- 1. Create principal_roles junction table
-- =============================================================================

CREATE TABLE principal_roles (
    principal_id VARCHAR(17) NOT NULL,
    role_name VARCHAR(100) NOT NULL,
    assignment_source VARCHAR(50),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (principal_id, role_name),
    FOREIGN KEY (principal_id) REFERENCES principals(id) ON DELETE CASCADE
);

-- Index for finding all principals with a specific role
CREATE INDEX idx_principal_roles_role_name ON principal_roles(role_name);

-- Index for finding when roles were assigned (useful for auditing)
CREATE INDEX idx_principal_roles_assigned_at ON principal_roles(assigned_at);

-- =============================================================================
-- 2. Migrate existing JSONB data (if column exists)
-- =============================================================================

-- Insert rows from the existing JSONB column
-- Handle assignedAt as either epoch milliseconds (numeric) or ISO timestamp string
DO $$
BEGIN
    -- Check if the roles column exists before migrating
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'principals' AND column_name = 'roles'
    ) THEN
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
    END IF;
END $$;

-- =============================================================================
-- 3. Drop the JSONB roles column
-- =============================================================================

ALTER TABLE principals DROP COLUMN IF EXISTS roles;
