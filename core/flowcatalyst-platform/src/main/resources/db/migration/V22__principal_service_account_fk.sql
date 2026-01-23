-- V22: Add service_account_id FK to principals table
-- This replaces the embedded service_account JSONB with a proper foreign key relationship.
-- A Principal of type SERVICE now references a ServiceAccount entity (1:1 relationship).

-- Add service_account_id column to principals
ALTER TABLE principals ADD COLUMN service_account_id VARCHAR(17);

-- Add index for efficient lookups
CREATE INDEX idx_principals_service_account_id ON principals(service_account_id);

-- Add unique constraint - each service account can only have one principal
ALTER TABLE principals ADD CONSTRAINT uq_principals_service_account_id UNIQUE (service_account_id);

-- Note: We keep the service_account JSONB column for backwards compatibility during migration.
-- It will be deprecated and eventually removed.

-- Migrate existing data: For any Principal with embedded service_account JSON that has a code,
-- try to find the matching ServiceAccount entity and link them.
-- This is a best-effort migration - manual review may be needed for edge cases.
UPDATE principals p
SET service_account_id = sa.id
FROM service_accounts sa
WHERE p.type = 'SERVICE'
  AND p.service_account_id IS NULL
  AND p.service_account IS NOT NULL
  AND sa.code = p.service_account->>'code';

-- Drop the service_account_roles table since roles will now be on the Principal
-- First, migrate any existing service account roles to the principal
INSERT INTO principal_roles (principal_id, role_name, assignment_source, assigned_at)
SELECT p.id, sar.role_name, sar.assignment_source, sar.assigned_at
FROM service_account_roles sar
JOIN service_accounts sa ON sa.id = sar.service_account_id
JOIN principals p ON p.service_account_id = sa.id
WHERE NOT EXISTS (
    SELECT 1 FROM principal_roles pr
    WHERE pr.principal_id = p.id AND pr.role_name = sar.role_name
);

-- We'll keep the service_account_roles table for now but it will be deprecated
-- and roles should be assigned via principal_roles going forward
