-- Optimize msg_dispatch_jobs indexes for write performance.
--
-- This table is high-throughput (batch inserts, frequent status transitions).
-- Indexes should cover the transactional processing paths only.
-- Rich query indexes belong on msg_dispatch_jobs_read (the projection).

-- 1. Add queued_at column for stale job detection
ALTER TABLE msg_dispatch_jobs ADD COLUMN IF NOT EXISTS queued_at TIMESTAMPTZ;

-- 2. Drop query-oriented indexes that slow down writes
--    (these access patterns are served by the read projection table)
DROP INDEX IF EXISTS idx_msg_dispatch_jobs_client_id;
DROP INDEX IF EXISTS idx_msg_dispatch_jobs_subscription_id;
DROP INDEX IF EXISTS idx_msg_dispatch_jobs_created_at;
DROP INDEX IF EXISTS idx_msg_dispatch_jobs_scheduled_for;
DROP INDEX IF EXISTS idx_msg_dispatch_jobs_status;
DROP INDEX IF EXISTS idx_msg_dispatch_jobs_message_group;

-- 3. Compound indexes for transactional processing paths

-- Scheduler poll: WHERE status = 'PENDING' ORDER BY message_group, sequence, created_at LIMIT N
-- Partial index — only rows the scheduler cares about, covers the ORDER BY
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_pending_poll
    ON msg_dispatch_jobs (message_group NULLS LAST, sequence, created_at)
    WHERE status = 'PENDING';

-- Block-on-error check: WHERE message_group IN (...) AND status IN ('FAILED', 'ERROR')
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_blocked_groups
    ON msg_dispatch_jobs (message_group, status)
    WHERE status IN ('FAILED', 'ERROR');

-- Stale recovery: WHERE status = 'QUEUED' AND queued_at < threshold
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_stale_queued
    ON msg_dispatch_jobs (queued_at)
    WHERE status = 'QUEUED';

-- NOTE: The following indexes are KEPT (not dropped):
-- - Primary key (id) — used by all single-job lookups and updates
-- - idx_msg_dispatch_jobs_unprojected — used by stream processor (created_at WHERE projected_at IS NULL)
