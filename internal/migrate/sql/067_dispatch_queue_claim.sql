-- +goose Up
-- Step 3 of the dispatch queue table: the scheduler claims from msg_dispatch_queue
-- (by claimed_at), so the dispatch path no longer reads PENDING jobs from
-- msg_dispatch_jobs and every partial index it relied on goes. No partial
-- indexes, no triggers. Identical in every FlowCatalyst implementation that
-- shares the database.

-- the dispatch path no longer reads PENDING jobs from msg_dispatch_jobs
DROP INDEX IF EXISTS idx_dispatch_jobs_pending_poll;
DROP INDEX IF EXISTS idx_dispatch_jobs_group_holders;
DROP INDEX IF EXISTS idx_dispatch_jobs_in_flight;
-- one ordinary index for every remaining status-based read: hold-back (FAILED/ERROR by group),
-- the stale sweeps and the reaper (QUEUED/PROCESSING), and the reconcile sweep (PENDING)
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_status_group
    ON msg_dispatch_jobs (status, message_group, sequence, created_at, id);
-- the queue table is small and churns: keep updates HOT and vacuum it by row count, not by ratio
ALTER TABLE msg_dispatch_queue SET (
    fillfactor = 70,
    autovacuum_vacuum_scale_factor = 0,  autovacuum_vacuum_threshold = 2000,
    autovacuum_analyze_scale_factor = 0, autovacuum_analyze_threshold = 2000);
-- start consistent: repair anything an older binary left behind
INSERT INTO msg_dispatch_queue (job_id, job_created_at, message_group, sequence, scheduled_for,
        subscription_id, dispatch_pool_id, client_id, mode, queue, version)
SELECT id, created_at, message_group, sequence, scheduled_for, subscription_id, dispatch_pool_id,
       client_id, mode, queue, updated_at
  FROM msg_dispatch_jobs WHERE status = 'PENDING'
ON CONFLICT (job_id) DO NOTHING;
DELETE FROM msg_dispatch_queue q
 WHERE NOT EXISTS (SELECT 1 FROM msg_dispatch_jobs j
                    WHERE j.id = q.job_id AND j.created_at = q.job_created_at AND j.status = 'PENDING');

-- +goose Down
DROP INDEX IF EXISTS idx_dispatch_jobs_status_group;
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_pending_poll
    ON msg_dispatch_jobs (message_group NULLS LAST, sequence, created_at, id)
    WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_group_holders
    ON msg_dispatch_jobs (message_group, sequence, created_at, id)
    WHERE message_group IS NOT NULL
      AND (status IN ('FAILED', 'ERROR') OR (status = 'PENDING' AND scheduled_for IS NOT NULL));
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_in_flight
    ON msg_dispatch_jobs (status, updated_at)
    WHERE status IN ('QUEUED', 'PROCESSING');
ALTER TABLE msg_dispatch_queue RESET (
    fillfactor,
    autovacuum_vacuum_scale_factor, autovacuum_vacuum_threshold,
    autovacuum_analyze_scale_factor, autovacuum_analyze_threshold);
