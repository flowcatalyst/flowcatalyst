-- +goose Up
-- The dispatch-job projector claims dirty rows with
--     WHERE projected_at IS NULL OR updated_at > projected_at
-- but idx_msg_dispatch_jobs_unprojected only covered the first arm, so every
-- re-projection of an UPDATED job (each status flip) fell back to scanning
-- the partitions. Replace it with a partial index on the full dirty
-- predicate — written verbatim in the claim query so the planner matches it.
-- The index stays tiny: it only ever holds the not-yet-projected rows.
DROP INDEX IF EXISTS idx_msg_dispatch_jobs_unprojected;
CREATE INDEX IF NOT EXISTS idx_msg_dispatch_jobs_dirty
    ON msg_dispatch_jobs (created_at)
    WHERE projected_at IS NULL OR updated_at > projected_at;

-- +goose Down
DROP INDEX IF EXISTS idx_msg_dispatch_jobs_dirty;
CREATE INDEX IF NOT EXISTS idx_msg_dispatch_jobs_unprojected
    ON msg_dispatch_jobs (created_at)
    WHERE projected_at IS NULL;
