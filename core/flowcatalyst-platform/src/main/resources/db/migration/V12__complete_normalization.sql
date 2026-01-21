-- V12: Complete normalization - add missing tables, alter existing, and drop JSONB columns
--
-- Note: dispatch_job_headers removed (headers calculated at dispatch time)
-- Note: dispatch_job_metadata kept as JSONB on dispatch_jobs table (not normalized)

-- =============================================================================
-- 1. Create service_account_client_ids table
-- =============================================================================
CREATE TABLE service_account_client_ids (
    id BIGSERIAL PRIMARY KEY,
    service_account_id VARCHAR(17) NOT NULL,
    client_id VARCHAR(17) NOT NULL
);

CREATE INDEX idx_service_account_client_ids_account ON service_account_client_ids(service_account_id);
CREATE INDEX idx_service_account_client_ids_client ON service_account_client_ids(client_id);

-- =============================================================================
-- 2. Create event_read_context_data table
-- =============================================================================
CREATE TABLE event_read_context_data (
    id BIGSERIAL PRIMARY KEY,
    event_read_id VARCHAR(13) NOT NULL,
    context_key VARCHAR(100) NOT NULL,
    context_value VARCHAR(1000)
);

CREATE INDEX idx_event_read_context_data_event ON event_read_context_data(event_read_id);
CREATE INDEX idx_event_read_context_data_key ON event_read_context_data(context_key);

-- =============================================================================
-- 3. Alter service_account_roles - add id column and assignment_source
-- =============================================================================
-- First drop the composite primary key
ALTER TABLE service_account_roles DROP CONSTRAINT service_account_roles_pkey;

-- Add the id column
ALTER TABLE service_account_roles ADD COLUMN id BIGSERIAL;

-- Make id the primary key
ALTER TABLE service_account_roles ADD PRIMARY KEY (id);

-- Add unique constraint for the original composite key
ALTER TABLE service_account_roles ADD CONSTRAINT service_account_roles_account_role_unique
    UNIQUE (service_account_id, role_name);

-- Add assignment_source column
ALTER TABLE service_account_roles ADD COLUMN assignment_source VARCHAR(50);

-- =============================================================================
-- 4. Alter dispatch_job_attempts - add missing columns
-- =============================================================================
-- Rename status_code to response_code
ALTER TABLE dispatch_job_attempts RENAME COLUMN status_code TO response_code;

-- Add missing columns
ALTER TABLE dispatch_job_attempts ADD COLUMN completed_at TIMESTAMPTZ;
ALTER TABLE dispatch_job_attempts ADD COLUMN response_body TEXT;
ALTER TABLE dispatch_job_attempts ADD COLUMN error_stack_trace TEXT;
ALTER TABLE dispatch_job_attempts ADD COLUMN error_type VARCHAR(20);
ALTER TABLE dispatch_job_attempts ADD COLUMN created_at TIMESTAMPTZ;

-- Make status nullable (entity doesn't require it)
ALTER TABLE dispatch_job_attempts ALTER COLUMN status DROP NOT NULL;
ALTER TABLE dispatch_job_attempts ALTER COLUMN attempt_number DROP NOT NULL;
ALTER TABLE dispatch_job_attempts ALTER COLUMN attempted_at DROP NOT NULL;

-- =============================================================================
-- 5. Drop JSONB/array columns (metadata kept on dispatch_jobs, headers removed from V1)
-- =============================================================================
ALTER TABLE principals DROP COLUMN IF EXISTS roles;
ALTER TABLE service_accounts DROP COLUMN IF EXISTS roles;
ALTER TABLE service_accounts DROP COLUMN IF EXISTS client_ids;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS event_types;
ALTER TABLE dispatch_jobs DROP COLUMN IF EXISTS attempts;
ALTER TABLE events_read DROP COLUMN IF EXISTS context_data;
