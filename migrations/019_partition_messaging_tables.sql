-- Partition the high-volume messaging tables by `created_at`, monthly.
--
-- Production-only migration. fc-dev (embedded postgres) skips this.
--
-- Why: cleanup at this throughput is not viable via DELETE — every batched
-- DELETE generates WAL equal to the deleted volume and competes with ingest
-- for IOPS. Range-partitioning by `created_at` and dropping old partitions
-- is O(1), zero WAL for the data, and removes vacuum pressure on the hot
-- write tables.
--
-- This migration assumes the messaging tables are essentially empty
-- (pre-launch / dev data only). It DROPs and recreates them on first run.
-- It is a no-op on subsequent runs — guarded by an explicit check that
-- msg_events is already partitioned.
--
-- Tables affected:
--   - msg_events
--   - msg_events_read
--   - msg_dispatch_jobs
--   - msg_dispatch_jobs_read
--   - msg_dispatch_job_attempts
--
-- All partitioned by RANGE (created_at), monthly granularity.
-- Initial partitions cover (current month - 1) through (current month + 12).
-- Forward partitions and retention drops are managed by the partition manager
-- service in fc-stream.
--
-- Constraints introduced by partitioning:
--   - Partition key must be NOT NULL → `created_at` made NOT NULL DEFAULT NOW().
--   - Primary key must include partition key → PK is (id, created_at).
--   - UNIQUE indexes must include partition key → UNIQUE on
--     (deduplication_id, created_at), per-partition. Two events with the
--     same deduplication_id in *different* months are no longer rejected
--     by the database. App-level dedup compensates if needed.

DO $migration018$
DECLARE
    already_partitioned boolean;
    parent_table TEXT;
    parents TEXT[] := ARRAY[
        'msg_events',
        'msg_events_read',
        'msg_dispatch_jobs',
        'msg_dispatch_jobs_read',
        'msg_dispatch_job_attempts'
    ];
    m INTEGER;
    months_back CONSTANT INTEGER := 1;
    months_forward CONSTANT INTEGER := 12;
    start_ts TIMESTAMPTZ;
    end_ts TIMESTAMPTZ;
    partition_name TEXT;
BEGIN
    -- Guard: skip if msg_events is already a partitioned table.
    SELECT EXISTS (
        SELECT 1
        FROM pg_partitioned_table pt
        JOIN pg_class c ON c.oid = pt.partrelid
        WHERE c.relname = 'msg_events'
    ) INTO already_partitioned;

    IF already_partitioned THEN
        RAISE NOTICE 'Migration 018: msg_events is already partitioned; skipping.';
        RETURN;
    END IF;

    -- ─── Drop existing (unpartitioned) tables ───────────────────────────────
    DROP TABLE IF EXISTS msg_events CASCADE;
    DROP TABLE IF EXISTS msg_events_read CASCADE;
    DROP TABLE IF EXISTS msg_dispatch_jobs CASCADE;
    DROP TABLE IF EXISTS msg_dispatch_jobs_read CASCADE;
    DROP TABLE IF EXISTS msg_dispatch_job_attempts CASCADE;

    -- ─── msg_events ─────────────────────────────────────────────────────────
    CREATE TABLE msg_events (
        id VARCHAR(13) NOT NULL,
        spec_version VARCHAR(20) DEFAULT '1.0',
        type VARCHAR(200) NOT NULL,
        source VARCHAR(500) NOT NULL,
        subject VARCHAR(500),
        time TIMESTAMPTZ NOT NULL,
        data JSONB,
        correlation_id VARCHAR(100),
        causation_id VARCHAR(100),
        deduplication_id VARCHAR(200),
        message_group VARCHAR(200),
        client_id VARCHAR(17),
        context_data JSONB,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        projected_at TIMESTAMPTZ,
        fanned_out_at TIMESTAMPTZ,
        PRIMARY KEY (id, created_at)
    ) PARTITION BY RANGE (created_at);

    CREATE INDEX idx_msg_events_client_id ON msg_events (client_id);
    CREATE INDEX idx_msg_events_created_at ON msg_events (created_at);
    CREATE INDEX idx_msg_events_unprojected ON msg_events (created_at) WHERE projected_at IS NULL;
    CREATE INDEX idx_msg_events_unfanned ON msg_events (created_at) WHERE fanned_out_at IS NULL;
    CREATE UNIQUE INDEX idx_msg_events_deduplication ON msg_events (deduplication_id, created_at);

    -- ─── msg_events_read ────────────────────────────────────────────────────
    CREATE TABLE msg_events_read (
        id VARCHAR(13) NOT NULL,
        spec_version VARCHAR(20),
        type VARCHAR(200) NOT NULL,
        source VARCHAR(500) NOT NULL,
        subject VARCHAR(500),
        time TIMESTAMPTZ NOT NULL,
        data TEXT,
        correlation_id VARCHAR(100),
        causation_id VARCHAR(100),
        deduplication_id VARCHAR(200),
        message_group VARCHAR(200),
        client_id VARCHAR(17),
        application VARCHAR(100),
        subdomain VARCHAR(100),
        aggregate VARCHAR(100),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        projected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        PRIMARY KEY (id, created_at)
    ) PARTITION BY RANGE (created_at);

    CREATE INDEX idx_msg_events_read_type ON msg_events_read (type);
    CREATE INDEX idx_msg_events_read_client_id ON msg_events_read (client_id);
    CREATE INDEX idx_msg_events_read_time ON msg_events_read (time);
    CREATE INDEX idx_msg_events_read_application ON msg_events_read (application);
    CREATE INDEX idx_msg_events_read_subdomain ON msg_events_read (subdomain);
    CREATE INDEX idx_msg_events_read_aggregate ON msg_events_read (aggregate);
    CREATE INDEX idx_msg_events_read_correlation_id ON msg_events_read (correlation_id);

    -- ─── msg_dispatch_jobs ──────────────────────────────────────────────────
    CREATE TABLE msg_dispatch_jobs (
        id VARCHAR(13) NOT NULL,
        external_id VARCHAR(100),
        source VARCHAR(500),
        kind VARCHAR(20) NOT NULL DEFAULT 'EVENT',
        code VARCHAR(200) NOT NULL,
        subject VARCHAR(500),
        event_id VARCHAR(13),
        correlation_id VARCHAR(100),
        metadata JSONB DEFAULT '[]'::jsonb,
        target_url VARCHAR(500) NOT NULL,
        protocol VARCHAR(30) NOT NULL DEFAULT 'HTTP_WEBHOOK',
        payload TEXT,
        payload_content_type VARCHAR(100) DEFAULT 'application/json',
        data_only BOOLEAN NOT NULL DEFAULT TRUE,
        service_account_id VARCHAR(17),
        client_id VARCHAR(17),
        subscription_id VARCHAR(17),
        mode VARCHAR(30) NOT NULL DEFAULT 'IMMEDIATE',
        dispatch_pool_id VARCHAR(17),
        message_group VARCHAR(200),
        sequence INTEGER NOT NULL DEFAULT 99,
        timeout_seconds INTEGER NOT NULL DEFAULT 30,
        schema_id VARCHAR(17),
        status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
        max_retries INTEGER NOT NULL DEFAULT 3,
        retry_strategy VARCHAR(50) DEFAULT 'exponential',
        scheduled_for TIMESTAMPTZ,
        expires_at TIMESTAMPTZ,
        attempt_count INTEGER NOT NULL DEFAULT 0,
        last_attempt_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        duration_millis BIGINT,
        last_error TEXT,
        idempotency_key VARCHAR(100),
        queued_at TIMESTAMPTZ,
        projected_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        PRIMARY KEY (id, created_at)
    ) PARTITION BY RANGE (created_at);

    CREATE INDEX idx_dispatch_jobs_pending_poll
        ON msg_dispatch_jobs (message_group NULLS LAST, sequence, created_at)
        WHERE status = 'PENDING';

    CREATE INDEX idx_dispatch_jobs_blocked_groups
        ON msg_dispatch_jobs (message_group, status)
        WHERE status IN ('FAILED', 'ERROR');

    CREATE INDEX idx_dispatch_jobs_stale_queued
        ON msg_dispatch_jobs (queued_at)
        WHERE status = 'QUEUED';

    CREATE INDEX idx_msg_dispatch_jobs_unprojected
        ON msg_dispatch_jobs (created_at) WHERE projected_at IS NULL;

    -- ─── msg_dispatch_jobs_read ─────────────────────────────────────────────
    CREATE TABLE msg_dispatch_jobs_read (
        id VARCHAR(13) NOT NULL,
        external_id VARCHAR(100),
        source VARCHAR(500),
        kind VARCHAR(20) NOT NULL,
        code VARCHAR(200) NOT NULL,
        subject VARCHAR(500),
        event_id VARCHAR(13),
        correlation_id VARCHAR(100),
        target_url VARCHAR(500) NOT NULL,
        protocol VARCHAR(30) NOT NULL,
        service_account_id VARCHAR(17),
        client_id VARCHAR(17),
        subscription_id VARCHAR(17),
        dispatch_pool_id VARCHAR(17),
        mode VARCHAR(30) NOT NULL,
        message_group VARCHAR(200),
        sequence INTEGER DEFAULT 99,
        timeout_seconds INTEGER DEFAULT 30,
        status VARCHAR(20) NOT NULL,
        max_retries INTEGER NOT NULL,
        retry_strategy VARCHAR(50),
        scheduled_for TIMESTAMPTZ,
        expires_at TIMESTAMPTZ,
        attempt_count INTEGER NOT NULL DEFAULT 0,
        last_attempt_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        duration_millis BIGINT,
        last_error TEXT,
        idempotency_key VARCHAR(100),
        is_completed BOOLEAN,
        is_terminal BOOLEAN,
        application VARCHAR(100),
        subdomain VARCHAR(100),
        aggregate VARCHAR(100),
        updated_at TIMESTAMPTZ NOT NULL,
        projected_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        PRIMARY KEY (id, created_at)
    ) PARTITION BY RANGE (created_at);

    CREATE INDEX idx_msg_dispatch_jobs_read_status ON msg_dispatch_jobs_read (status);
    CREATE INDEX idx_msg_dispatch_jobs_read_client_id ON msg_dispatch_jobs_read (client_id);
    CREATE INDEX idx_msg_dispatch_jobs_read_application ON msg_dispatch_jobs_read (application);
    CREATE INDEX idx_msg_dispatch_jobs_read_subscription_id ON msg_dispatch_jobs_read (subscription_id);
    CREATE INDEX idx_msg_dispatch_jobs_read_message_group ON msg_dispatch_jobs_read (message_group);
    CREATE INDEX idx_msg_dispatch_jobs_read_created_at ON msg_dispatch_jobs_read (created_at);

    -- ─── msg_dispatch_job_attempts ──────────────────────────────────────────
    CREATE TABLE msg_dispatch_job_attempts (
        id VARCHAR(13) NOT NULL,
        dispatch_job_id VARCHAR(13) NOT NULL,
        attempt_number INTEGER,
        status VARCHAR(20),
        response_code INTEGER,
        response_body TEXT,
        error_message TEXT,
        error_stack_trace TEXT,
        error_type VARCHAR(20),
        duration_millis BIGINT,
        attempted_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        PRIMARY KEY (id, created_at)
    ) PARTITION BY RANGE (created_at);

    CREATE UNIQUE INDEX idx_msg_dispatch_job_attempts_job_number
        ON msg_dispatch_job_attempts (dispatch_job_id, attempt_number, created_at);

    CREATE INDEX idx_msg_dispatch_job_attempts_job
        ON msg_dispatch_job_attempts (dispatch_job_id);

    -- ─── Initial partitions ────────────────────────────────────────────────
    -- Bootstrap monthly partitions covering (this month - 1) through
    -- (this month + 12). The partition manager (fc-stream) handles forward
    -- rolling and retention drops.
    FOREACH parent_table IN ARRAY parents LOOP
        FOR m IN -months_back..months_forward LOOP
            start_ts := date_trunc('month', NOW()) + (m || ' months')::INTERVAL;
            end_ts := start_ts + INTERVAL '1 month';
            partition_name := parent_table || '_' || to_char(start_ts, 'YYYY_MM');

            EXECUTE format(
                'CREATE TABLE IF NOT EXISTS %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
                partition_name,
                parent_table,
                start_ts,
                end_ts
            );
        END LOOP;
    END LOOP;
END
$migration018$;
