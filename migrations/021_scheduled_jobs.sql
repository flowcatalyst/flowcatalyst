-- Scheduled Jobs: cron-triggered webhook jobs with optional completion callback.
--
-- Tables:
--   msg_scheduled_jobs              — definition aggregate (regular table, low volume)
--   msg_scheduled_job_instances     — per-firing history (composite PK, partitioned in 022)
--   msg_scheduled_job_instance_logs — per-instance log entries (composite PK, partitioned in 022)
--
-- The instance + log tables use composite PRIMARY KEY (id, created_at) so that
-- the production migration (022) can re-create them as RANGE-partitioned by
-- created_at without an app-side schema change.
--
-- All statements use IF NOT EXISTS — re-running this migration is a no-op.

-- ─── Definitions ────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS msg_scheduled_jobs (
    id VARCHAR(17) PRIMARY KEY,
    client_id VARCHAR(17),
    code VARCHAR(200) NOT NULL,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    crons TEXT[] NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
    payload JSONB,
    concurrent BOOLEAN NOT NULL DEFAULT FALSE,
    tracks_completion BOOLEAN NOT NULL DEFAULT FALSE,
    timeout_seconds INTEGER,
    delivery_max_attempts INTEGER NOT NULL DEFAULT 3,
    target_url VARCHAR(500),
    last_fired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(17),
    updated_by VARCHAR(17),
    version INTEGER NOT NULL DEFAULT 1
);

-- Tenant-scoped uniqueness on code (only when client_id is set)
CREATE UNIQUE INDEX IF NOT EXISTS idx_msg_scheduled_jobs_code_per_client
    ON msg_scheduled_jobs (client_id, code) WHERE client_id IS NOT NULL;

-- Platform-scoped uniqueness on code (when client_id IS NULL)
CREATE UNIQUE INDEX IF NOT EXISTS idx_msg_scheduled_jobs_code_platform
    ON msg_scheduled_jobs (code) WHERE client_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_msg_scheduled_jobs_client_id
    ON msg_scheduled_jobs (client_id);

-- Poller hot path: only ACTIVE jobs are polled, ordered by oldest last_fired_at.
CREATE INDEX IF NOT EXISTS idx_msg_scheduled_jobs_active_poll
    ON msg_scheduled_jobs (last_fired_at NULLS FIRST) WHERE status = 'ACTIVE';

-- ─── Instances ──────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS msg_scheduled_job_instances (
    id VARCHAR(17) NOT NULL,
    scheduled_job_id VARCHAR(17) NOT NULL,
    client_id VARCHAR(17),
    job_code VARCHAR(200) NOT NULL,
    trigger_kind VARCHAR(20) NOT NULL DEFAULT 'CRON',
    scheduled_for TIMESTAMPTZ,
    fired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL DEFAULT 'QUEUED',
    delivery_attempts INTEGER NOT NULL DEFAULT 0,
    delivery_error TEXT,
    completion_status VARCHAR(20),
    completion_result JSONB,
    correlation_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
);

CREATE INDEX IF NOT EXISTS idx_msg_scheduled_job_instances_job
    ON msg_scheduled_job_instances (scheduled_job_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_msg_scheduled_job_instances_client
    ON msg_scheduled_job_instances (client_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_msg_scheduled_job_instances_status
    ON msg_scheduled_job_instances (status, created_at);

-- Overlap detection (non-terminal). Partial → small index even at high volume.
CREATE INDEX IF NOT EXISTS idx_msg_scheduled_job_instances_active
    ON msg_scheduled_job_instances (scheduled_job_id)
    WHERE status IN ('QUEUED', 'IN_FLIGHT', 'DELIVERED');

-- ─── Instance logs ──────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS msg_scheduled_job_instance_logs (
    id VARCHAR(17) NOT NULL,
    instance_id VARCHAR(17) NOT NULL,
    scheduled_job_id VARCHAR(17),
    client_id VARCHAR(17),
    level VARCHAR(10) NOT NULL DEFAULT 'INFO',
    message TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
);

CREATE INDEX IF NOT EXISTS idx_msg_scheduled_job_instance_logs_instance
    ON msg_scheduled_job_instance_logs (instance_id, created_at);

CREATE INDEX IF NOT EXISTS idx_msg_scheduled_job_instance_logs_job
    ON msg_scheduled_job_instance_logs (scheduled_job_id, created_at DESC);
