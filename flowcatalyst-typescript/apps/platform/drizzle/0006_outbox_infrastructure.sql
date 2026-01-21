-- V6: Outbox infrastructure for CQRS read model projection
--
-- Pattern: Application-level concurrency control with message groups
-- - Single poller fetches batches without DB locks
-- - dispatch_job_id serves as message group for sequencing
-- - Payload captured at write time for consistency (no fetch-at-projection-time)

-- =============================================================================
-- 1. Create dispatch_job_outbox table
-- =============================================================================
-- Payload-based outbox: captures state at write time
-- - INSERT: full job payload
-- - UPDATE: patch with changed fields only
-- - DELETE: just the operation marker
--
-- processed values (following postbox-processor pattern):
--   0 = pending
--   1 = success
--   2 = bad request (permanent failure)
--   3 = server error (retriable)
--   9 = in-progress (crash recovery marker)

CREATE TABLE dispatch_job_outbox (
    id BIGSERIAL PRIMARY KEY,
    dispatch_job_id VARCHAR(17) NOT NULL,
    operation VARCHAR(10) NOT NULL,      -- INSERT, UPDATE, DELETE
    payload JSONB NOT NULL,              -- Full job on INSERT, patch on UPDATE
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed SMALLINT NOT NULL DEFAULT 0,
    processed_at TIMESTAMPTZ,
    error_message TEXT
);

-- Index for polling unprocessed entries, ordered by job (message group) then sequence
CREATE INDEX idx_dispatch_job_outbox_unprocessed
    ON dispatch_job_outbox(dispatch_job_id, id)
    WHERE processed = 0;

-- Index for crash recovery (find in-progress entries)
CREATE INDEX idx_dispatch_job_outbox_in_progress
    ON dispatch_job_outbox(id)
    WHERE processed = 9;

-- Index for cleanup of old processed entries
CREATE INDEX idx_dispatch_job_outbox_processed_at
    ON dispatch_job_outbox(processed_at)
    WHERE processed = 1;

-- =============================================================================
-- 2. Create event_outbox table
-- =============================================================================
-- Events are simpler (immutable), but using outbox for:
-- - Consistent processing pattern
-- - Crash recovery
-- - Error tracking

CREATE TABLE event_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(17) NOT NULL,
    payload JSONB NOT NULL,              -- Full event data
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed SMALLINT NOT NULL DEFAULT 0,
    processed_at TIMESTAMPTZ,
    error_message TEXT
);

CREATE INDEX idx_event_outbox_unprocessed
    ON event_outbox(id)
    WHERE processed = 0;

CREATE INDEX idx_event_outbox_in_progress
    ON event_outbox(id)
    WHERE processed = 9;

-- =============================================================================
-- 3. Comments for documentation
-- =============================================================================

COMMENT ON TABLE dispatch_job_outbox IS 'Outbox for CQRS projection of dispatch_jobs to dispatch_jobs_read. Uses dispatch_job_id as message group for sequencing. Payload captured at write time.';
COMMENT ON COLUMN dispatch_job_outbox.processed IS '0=pending, 1=success, 2=bad_request, 3=server_error, 9=in_progress';
COMMENT ON COLUMN dispatch_job_outbox.operation IS 'INSERT=full payload, UPDATE=patch with changed fields, DELETE=tombstone';
COMMENT ON COLUMN dispatch_job_outbox.payload IS 'JSON: full object on INSERT, changed fields only on UPDATE';

COMMENT ON TABLE event_outbox IS 'Outbox for CQRS projection of events to events_read. Full event payload captured at write time.';
