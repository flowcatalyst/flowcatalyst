-- Reshape the messaging tables to the partitioning-ready schema.
--
-- Runs on every profile, including the embedded fc-dev DB. It puts the
-- tables into the shape the Rust code now expects (composite primary key
-- on (id, created_at), composite UNIQUE on deduplication_id, fanned_out_at
-- on events, created_at on the read tables). The actual `PARTITION BY
-- RANGE (created_at)` only happens in migration 019, which is production-
-- only; on embedded the tables stay regular tables with this new shape.
--
-- Idempotent: every alter is guarded so a re-run is a no-op.

-- ─── msg_events ───────────────────────────────────────────────────────────

ALTER TABLE msg_events ADD COLUMN IF NOT EXISTS fanned_out_at TIMESTAMPTZ;

ALTER TABLE msg_events ALTER COLUMN created_at SET NOT NULL;

DO $events$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'msg_events_pkey' AND conrelid = 'msg_events'::regclass
    ) AND NOT EXISTS (
        SELECT 1 FROM pg_index i
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
        WHERE i.indrelid = 'msg_events'::regclass
          AND i.indisprimary
          AND a.attname = 'created_at'
    ) THEN
        ALTER TABLE msg_events DROP CONSTRAINT msg_events_pkey;
        ALTER TABLE msg_events ADD CONSTRAINT msg_events_pkey PRIMARY KEY (id, created_at);
    END IF;
END
$events$;

DROP INDEX IF EXISTS idx_msg_events_type;
DROP INDEX IF EXISTS idx_msg_events_client_type;
DROP INDEX IF EXISTS idx_msg_events_time;
DROP INDEX IF EXISTS idx_msg_events_correlation;
DROP INDEX IF EXISTS idx_msg_events_deduplication;

CREATE INDEX IF NOT EXISTS idx_msg_events_client_id ON msg_events (client_id);
CREATE INDEX IF NOT EXISTS idx_msg_events_created_at ON msg_events (created_at);
CREATE INDEX IF NOT EXISTS idx_msg_events_unfanned ON msg_events (created_at) WHERE fanned_out_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_msg_events_deduplication ON msg_events (deduplication_id, created_at);

-- ─── msg_events_read ──────────────────────────────────────────────────────

ALTER TABLE msg_events_read ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

DO $events_read$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'msg_events_read_pkey' AND conrelid = 'msg_events_read'::regclass
    ) AND NOT EXISTS (
        SELECT 1 FROM pg_index i
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
        WHERE i.indrelid = 'msg_events_read'::regclass
          AND i.indisprimary
          AND a.attname = 'created_at'
    ) THEN
        ALTER TABLE msg_events_read DROP CONSTRAINT msg_events_read_pkey;
        ALTER TABLE msg_events_read ADD CONSTRAINT msg_events_read_pkey PRIMARY KEY (id, created_at);
    END IF;
END
$events_read$;

-- ─── msg_dispatch_jobs ────────────────────────────────────────────────────

ALTER TABLE msg_dispatch_jobs ALTER COLUMN created_at SET NOT NULL;

DO $dj$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'msg_dispatch_jobs_pkey' AND conrelid = 'msg_dispatch_jobs'::regclass
    ) AND NOT EXISTS (
        SELECT 1 FROM pg_index i
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
        WHERE i.indrelid = 'msg_dispatch_jobs'::regclass
          AND i.indisprimary
          AND a.attname = 'created_at'
    ) THEN
        ALTER TABLE msg_dispatch_jobs DROP CONSTRAINT msg_dispatch_jobs_pkey;
        ALTER TABLE msg_dispatch_jobs ADD CONSTRAINT msg_dispatch_jobs_pkey PRIMARY KEY (id, created_at);
    END IF;
END
$dj$;

-- ─── msg_dispatch_jobs_read ───────────────────────────────────────────────

ALTER TABLE msg_dispatch_jobs_read ALTER COLUMN created_at SET NOT NULL;

DO $djr$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'msg_dispatch_jobs_read_pkey' AND conrelid = 'msg_dispatch_jobs_read'::regclass
    ) AND NOT EXISTS (
        SELECT 1 FROM pg_index i
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
        WHERE i.indrelid = 'msg_dispatch_jobs_read'::regclass
          AND i.indisprimary
          AND a.attname = 'created_at'
    ) THEN
        ALTER TABLE msg_dispatch_jobs_read DROP CONSTRAINT msg_dispatch_jobs_read_pkey;
        ALTER TABLE msg_dispatch_jobs_read ADD CONSTRAINT msg_dispatch_jobs_read_pkey PRIMARY KEY (id, created_at);
    END IF;
END
$djr$;

-- ─── msg_dispatch_job_attempts ────────────────────────────────────────────

ALTER TABLE msg_dispatch_job_attempts ALTER COLUMN created_at SET DEFAULT NOW();
UPDATE msg_dispatch_job_attempts SET created_at = NOW() WHERE created_at IS NULL;
ALTER TABLE msg_dispatch_job_attempts ALTER COLUMN created_at SET NOT NULL;

DO $att$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'msg_dispatch_job_attempts_pkey' AND conrelid = 'msg_dispatch_job_attempts'::regclass
    ) AND NOT EXISTS (
        SELECT 1 FROM pg_index i
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
        WHERE i.indrelid = 'msg_dispatch_job_attempts'::regclass
          AND i.indisprimary
          AND a.attname = 'created_at'
    ) THEN
        ALTER TABLE msg_dispatch_job_attempts DROP CONSTRAINT msg_dispatch_job_attempts_pkey;
        ALTER TABLE msg_dispatch_job_attempts ADD CONSTRAINT msg_dispatch_job_attempts_pkey PRIMARY KEY (id, created_at);
    END IF;
END
$att$;

DROP INDEX IF EXISTS idx_msg_dispatch_job_attempts_job_number;
CREATE UNIQUE INDEX IF NOT EXISTS idx_msg_dispatch_job_attempts_job_number
    ON msg_dispatch_job_attempts (dispatch_job_id, attempt_number, created_at);
