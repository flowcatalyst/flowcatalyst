-- Partition msg_scheduled_job_instances and msg_scheduled_job_instance_logs
-- by RANGE (created_at), monthly. Mirrors the dispatch-job partitioning in 019.
--
-- Production-only. Drops and recreates these tables — assumed empty (i.e.,
-- this migration is applied either pre-launch or immediately after 021
-- introduces the unpartitioned tables, before any traffic hits them).
--
-- Idempotent: skips if msg_scheduled_job_instances is already partitioned.
--
-- Forward partitions and retention drops are managed by the partition manager
-- service in fc-stream (PARTITIONED_PARENTS const).

DO $migration022$
DECLARE
    already_partitioned boolean;
    parent_table TEXT;
    parents TEXT[] := ARRAY[
        'msg_scheduled_job_instances',
        'msg_scheduled_job_instance_logs'
    ];
    m INTEGER;
    months_back CONSTANT INTEGER := 1;
    months_forward CONSTANT INTEGER := 12;
    start_ts TIMESTAMPTZ;
    end_ts TIMESTAMPTZ;
    partition_name TEXT;
BEGIN
    SELECT EXISTS (
        SELECT 1
        FROM pg_partitioned_table pt
        JOIN pg_class c ON c.oid = pt.partrelid
        WHERE c.relname = 'msg_scheduled_job_instances'
    ) INTO already_partitioned;

    IF already_partitioned THEN
        RAISE NOTICE 'Migration 022: scheduled-job tables already partitioned; skipping.';
        RETURN;
    END IF;

    DROP TABLE IF EXISTS msg_scheduled_job_instances CASCADE;
    DROP TABLE IF EXISTS msg_scheduled_job_instance_logs CASCADE;

    -- ─── msg_scheduled_job_instances (partitioned) ─────────────────────────
    CREATE TABLE msg_scheduled_job_instances (
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
    ) PARTITION BY RANGE (created_at);

    CREATE INDEX idx_msg_scheduled_job_instances_job
        ON msg_scheduled_job_instances (scheduled_job_id, created_at DESC);

    CREATE INDEX idx_msg_scheduled_job_instances_client
        ON msg_scheduled_job_instances (client_id, created_at DESC);

    CREATE INDEX idx_msg_scheduled_job_instances_status
        ON msg_scheduled_job_instances (status, created_at);

    CREATE INDEX idx_msg_scheduled_job_instances_active
        ON msg_scheduled_job_instances (scheduled_job_id)
        WHERE status IN ('QUEUED', 'IN_FLIGHT', 'DELIVERED');

    -- ─── msg_scheduled_job_instance_logs (partitioned) ─────────────────────
    CREATE TABLE msg_scheduled_job_instance_logs (
        id VARCHAR(17) NOT NULL,
        instance_id VARCHAR(17) NOT NULL,
        scheduled_job_id VARCHAR(17),
        client_id VARCHAR(17),
        level VARCHAR(10) NOT NULL DEFAULT 'INFO',
        message TEXT NOT NULL,
        metadata JSONB,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        PRIMARY KEY (id, created_at)
    ) PARTITION BY RANGE (created_at);

    CREATE INDEX idx_msg_scheduled_job_instance_logs_instance
        ON msg_scheduled_job_instance_logs (instance_id, created_at);

    CREATE INDEX idx_msg_scheduled_job_instance_logs_job
        ON msg_scheduled_job_instance_logs (scheduled_job_id, created_at DESC);

    -- ─── Initial partitions ────────────────────────────────────────────────
    -- Bootstrap monthly partitions covering (this month - 1) through (+12).
    -- The partition manager handles forward rolling and retention drops.
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
$migration022$;
