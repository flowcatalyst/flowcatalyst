-- V5: Drop all foreign key constraints
-- Foreign keys are removed to allow more flexible data management and avoid
-- cascade issues. Indexes are retained/added for query performance.

-- =============================================================================
-- Drop Foreign Key Constraints
-- =============================================================================

-- principals.client_id -> clients(id)
ALTER TABLE principals DROP CONSTRAINT IF EXISTS principals_client_id_fkey;

-- service_accounts.application_id -> applications(id)
ALTER TABLE service_accounts DROP CONSTRAINT IF EXISTS service_accounts_application_id_fkey;

-- events.client_id -> clients(id)
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_client_id_fkey;

-- events_read.client_id -> clients(id)
ALTER TABLE events_read DROP CONSTRAINT IF EXISTS events_read_client_id_fkey;

-- subscriptions.client_id -> clients(id)
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_client_id_fkey;

-- dispatch_pools.client_id -> clients(id)
ALTER TABLE dispatch_pools DROP CONSTRAINT IF EXISTS dispatch_pools_client_id_fkey;

-- dispatch_jobs.client_id -> clients(id)
ALTER TABLE dispatch_jobs DROP CONSTRAINT IF EXISTS dispatch_jobs_client_id_fkey;

-- schemas.event_type_id -> event_types(id)
ALTER TABLE schemas DROP CONSTRAINT IF EXISTS schemas_event_type_id_fkey;

-- client_auth_configs.client_id -> clients(id)
ALTER TABLE client_auth_configs DROP CONSTRAINT IF EXISTS client_auth_configs_client_id_fkey;

-- client_auth_configs.primary_client_id -> clients(id)
ALTER TABLE client_auth_configs DROP CONSTRAINT IF EXISTS client_auth_configs_primary_client_id_fkey;

-- application_client_configs.application_id -> applications(id)
ALTER TABLE application_client_configs DROP CONSTRAINT IF EXISTS application_client_configs_application_id_fkey;

-- application_client_configs.client_id -> clients(id)
ALTER TABLE application_client_configs DROP CONSTRAINT IF EXISTS application_client_configs_client_id_fkey;

-- client_access_grants.principal_id -> principals(id)
ALTER TABLE client_access_grants DROP CONSTRAINT IF EXISTS client_access_grants_principal_id_fkey;

-- client_access_grants.client_id -> clients(id)
ALTER TABLE client_access_grants DROP CONSTRAINT IF EXISTS client_access_grants_client_id_fkey;

-- =============================================================================
-- Add Missing Indexes (columns that had FK but no index)
-- =============================================================================

-- client_auth_configs.client_id (deprecated column, but index for queries)
CREATE INDEX IF NOT EXISTS idx_client_auth_configs_client_id ON client_auth_configs(client_id);

-- client_auth_configs.primary_client_id
CREATE INDEX IF NOT EXISTS idx_client_auth_configs_primary_client_id ON client_auth_configs(primary_client_id);

-- =============================================================================
-- Add Unique Constraint on Email
-- =============================================================================

-- Prevent duplicate users with the same email (fixes race condition in OIDC login)
-- Drop the existing non-unique index first
DROP INDEX IF EXISTS idx_principals_email;

-- Create unique index on email extracted from JSONB
CREATE UNIQUE INDEX idx_principals_email_unique
ON principals((user_identity->>'email'))
WHERE user_identity->>'email' IS NOT NULL;
