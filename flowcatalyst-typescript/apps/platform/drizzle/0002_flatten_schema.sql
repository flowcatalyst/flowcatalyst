-- FlowCatalyst Platform Schema Refactor
-- Flatten JSONB columns to match Java schema structure
-- V2: Flatten principals.user_identity and add roles JSONB

-- =============================================================================
-- Part 1: Flatten user_identity into principals table
-- =============================================================================

-- Add flattened user identity columns to principals
ALTER TABLE principals ADD COLUMN IF NOT EXISTS email VARCHAR(255);
ALTER TABLE principals ADD COLUMN IF NOT EXISTS email_domain VARCHAR(100);
ALTER TABLE principals ADD COLUMN IF NOT EXISTS idp_type VARCHAR(50);
ALTER TABLE principals ADD COLUMN IF NOT EXISTS external_idp_id VARCHAR(255);
ALTER TABLE principals ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255);
ALTER TABLE principals ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP WITH TIME ZONE;

-- Add service_account JSONB column
ALTER TABLE principals ADD COLUMN IF NOT EXISTS service_account JSONB;

-- Add roles JSONB column
ALTER TABLE principals ADD COLUMN IF NOT EXISTS roles JSONB DEFAULT '[]'::jsonb;

-- Migrate data from user_identities to principals
-- Handle potential epoch timestamp format for last_login_at
UPDATE principals p
SET
    email = ui.email,
    email_domain = ui.email_domain,
    idp_type = ui.idp_type::VARCHAR,
    external_idp_id = ui.external_idp_id,
    password_hash = ui.password_hash,
    last_login_at = ui.last_login_at
FROM user_identities ui
WHERE p.id = ui.principal_id;

-- Migrate roles from role_assignments to principals.roles JSONB
UPDATE principals p
SET roles = (
    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'roleName', ra.role_name,
                'assignmentSource', ra.assignment_source,
                'assignedAt', ra.assigned_at
            )
        ),
        '[]'::jsonb
    )
    FROM role_assignments ra
    WHERE ra.principal_id = p.id
);

-- =============================================================================
-- Part 2: Update principals table to use VARCHAR instead of enums
-- =============================================================================

-- Convert type column from enum to varchar
ALTER TABLE principals
    ALTER COLUMN type TYPE VARCHAR(20) USING type::VARCHAR;

-- Convert scope column from enum to varchar
ALTER TABLE principals
    ALTER COLUMN scope TYPE VARCHAR(20) USING scope::VARCHAR;

-- =============================================================================
-- Part 3: Update indexes
-- =============================================================================

-- Drop old indexes
DROP INDEX IF EXISTS principals_type_idx;
DROP INDEX IF EXISTS principals_client_id_idx;
DROP INDEX IF EXISTS principals_active_idx;
DROP INDEX IF EXISTS user_identities_email_idx;
DROP INDEX IF EXISTS user_identities_email_domain_idx;
DROP INDEX IF EXISTS role_assignments_principal_id_idx;
DROP INDEX IF EXISTS role_assignments_role_name_idx;

-- Create new indexes matching Java schema naming
CREATE INDEX IF NOT EXISTS idx_principals_type ON principals(type);
CREATE INDEX IF NOT EXISTS idx_principals_client_id ON principals(client_id);
CREATE INDEX IF NOT EXISTS idx_principals_active ON principals(active);
CREATE UNIQUE INDEX IF NOT EXISTS idx_principals_email ON principals(email) WHERE email IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_principals_email_domain ON principals(email_domain) WHERE email_domain IS NOT NULL;

-- =============================================================================
-- Part 4: Drop old tables
-- =============================================================================

DROP TABLE IF EXISTS user_identities;
DROP TABLE IF EXISTS role_assignments;

-- =============================================================================
-- Part 5: Update clients table
-- =============================================================================

-- Convert status column from enum to varchar
ALTER TABLE clients
    ALTER COLUMN status TYPE VARCHAR(50) USING status::VARCHAR;

-- Update identifier length to match Java schema
ALTER TABLE clients
    ALTER COLUMN identifier TYPE VARCHAR(100);

-- Update status_reason length to match Java schema
ALTER TABLE clients
    ALTER COLUMN status_reason TYPE VARCHAR(255);

-- Update index names
DROP INDEX IF EXISTS clients_identifier_idx;
DROP INDEX IF EXISTS clients_status_idx;
CREATE INDEX IF NOT EXISTS idx_clients_identifier ON clients(identifier);
CREATE INDEX IF NOT EXISTS idx_clients_status ON clients(status);

-- =============================================================================
-- Part 6: Drop unused enums
-- =============================================================================

DROP TYPE IF EXISTS principal_type;
DROP TYPE IF EXISTS user_scope;
DROP TYPE IF EXISTS idp_type;
DROP TYPE IF EXISTS client_status;
