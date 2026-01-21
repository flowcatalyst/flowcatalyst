-- V17: Dispatch job changes table for linear state projection
--
-- Captures state transitions for dispatch jobs as JSONB patches.
-- Projector reads these linearly (by id) and applies to dispatch_jobs_read.

CREATE TABLE dispatch_job_changes (
    id BIGSERIAL PRIMARY KEY,
    dispatch_job_id VARCHAR(17) NOT NULL,
    operation VARCHAR(10) NOT NULL,  -- 'INSERT' or 'UPDATE'
    changes JSONB NOT NULL,          -- Full job for INSERT, patch for UPDATE
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    projected BOOLEAN NOT NULL DEFAULT false
);

-- Partial index for efficient polling of unprojected changes
CREATE INDEX idx_dispatch_job_changes_unprojected
    ON dispatch_job_changes(id)
    WHERE projected = false;

-- Index for looking up changes by job (for debugging/auditing)
CREATE INDEX idx_dispatch_job_changes_job_id
    ON dispatch_job_changes(dispatch_job_id);

COMMENT ON TABLE dispatch_job_changes IS 'Linear change log for dispatch jobs. INSERT = full job snapshot, UPDATE = JSONB patch of changed fields.';
COMMENT ON COLUMN dispatch_job_changes.operation IS 'INSERT for new jobs, UPDATE for state changes';
COMMENT ON COLUMN dispatch_job_changes.changes IS 'Full job JSON for INSERT, or patch with only changed fields for UPDATE';
