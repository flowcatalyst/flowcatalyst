-- ============================================================================
-- Add Connection entity
-- Connections sit between ServiceAccount and Subscription, providing a named
-- endpoint + credentials grouping with pause/unpause semantics.
-- ============================================================================

-- 1. Create msg_connections table
CREATE TABLE IF NOT EXISTS msg_connections (
    id varchar(17) PRIMARY KEY,
    code varchar(100) NOT NULL,
    name varchar(255) NOT NULL,
    description varchar(500),
    endpoint varchar(500) NOT NULL,
    external_id varchar(100),
    status varchar(20) NOT NULL DEFAULT 'ACTIVE',
    service_account_id varchar(17) NOT NULL,
    client_id varchar(17),
    client_identifier varchar(100),
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_msg_connections_code_client ON msg_connections (code, client_id);
CREATE INDEX IF NOT EXISTS idx_msg_connections_status ON msg_connections (status);
CREATE INDEX IF NOT EXISTS idx_msg_connections_client_id ON msg_connections (client_id);
CREATE INDEX IF NOT EXISTS idx_msg_connections_service_account ON msg_connections (service_account_id);

-- 2. Add connection_id to msg_dispatch_jobs
ALTER TABLE msg_dispatch_jobs ADD COLUMN IF NOT EXISTS connection_id varchar(17);
CREATE INDEX IF NOT EXISTS idx_msg_dispatch_jobs_connection_id ON msg_dispatch_jobs (connection_id);

-- 3. Add connection_id to msg_dispatch_jobs_read
ALTER TABLE msg_dispatch_jobs_read ADD COLUMN IF NOT EXISTS connection_id varchar(17);

-- 4. Add connection_id to msg_subscriptions (nullable initially for data migration)
ALTER TABLE msg_subscriptions ADD COLUMN IF NOT EXISTS connection_id varchar(17);
CREATE INDEX IF NOT EXISTS idx_msg_subscriptions_connection_id ON msg_subscriptions (connection_id);

-- 5. Truncate existing subscriptions (dev environment — no production data to migrate)
TRUNCATE TABLE msg_subscriptions CASCADE;
