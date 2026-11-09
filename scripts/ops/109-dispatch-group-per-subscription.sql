-- Per-subscription message groups: OPTIONAL re-key of jobs raised before the change.
--
-- Since the 2026-11-09 dispatch fixes, the event fan-out stores a job's message group as
-- "<subscription id>:<event message group>" (internal/platform/dispatchjob/group_key.go),
-- so ordering and a BLOCK_ON_ERROR hold are per subscriber. Jobs raised before that keep
-- the bare event group. They are delivered correctly, but in a different group from the
-- same subscription's new jobs: an old FAILED BLOCK_ON_ERROR job no longer holds that
-- subscription's new jobs, and old PENDING jobs are not ordered against new ones.
--
-- This script re-keys those old jobs to the new form, so they join their subscription's
-- new group. Only event fan-out jobs (idempotency_key = event_id:subscription_id) that are
-- PENDING or FAILED (ERROR) are touched: QUEUED and PROCESSING jobs are in flight under
-- the old key and finish under it; settled jobs need no group. updated_at is moved so the
-- read model (msg_dispatch_jobs_read) follows. A PENDING job the scheduler has just
-- claimed may then be published a second time; the router and the delivery callback
-- drop the duplicate.
--
-- Run AFTER every platform instance runs the new fan-out (an old instance would keep
-- writing bare groups), with psql, outside a transaction (the DO block commits per batch):
--     psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/ops/109-dispatch-group-per-subscription.sql
-- Re-runnable: a job already carrying its subscription prefix is skipped. Batches of
-- 5,000 rows, a commit and a short pause between them. Skip it entirely where few such
-- jobs exist (count first:
--     SELECT status, count(*) FROM msg_dispatch_jobs
--      WHERE status IN ('PENDING','FAILED','ERROR') AND subscription_id IS NOT NULL
--        AND message_group IS NOT NULL AND idempotency_key = event_id || ':' || subscription_id
--        AND message_group NOT LIKE subscription_id || ':%' GROUP BY status;).
SET lock_timeout = '5s';

DO $$
DECLARE
    moved integer;
    total integer := 0;
BEGIN
    LOOP
        WITH batch AS (
            SELECT id, created_at
              FROM msg_dispatch_jobs
             WHERE status IN ('PENDING', 'FAILED', 'ERROR')
               AND subscription_id IS NOT NULL
               AND message_group IS NOT NULL
               AND idempotency_key = event_id || ':' || subscription_id
               AND message_group NOT LIKE subscription_id || ':%'
             LIMIT 5000
        )
        UPDATE msg_dispatch_jobs j
           SET message_group = CASE
                   -- Same rule as dispatchjob.SubscriptionMessageGroup: the plain form when it
                   -- fits the 200-character column, else the SHA-256 digest of the event group.
                   WHEN length(j.subscription_id) + 1 + length(j.message_group) <= 200
                       THEN j.subscription_id || ':' || j.message_group
                   ELSE j.subscription_id || ':sha256:' || encode(sha256(convert_to(j.message_group, 'UTF8')), 'hex')
               END,
               updated_at = NOW()
          FROM batch b
         WHERE j.id = b.id AND j.created_at = b.created_at;
        GET DIAGNOSTICS moved = ROW_COUNT;
        EXIT WHEN moved = 0;
        total := total + moved;
        RAISE NOTICE 're-keyed % jobs (% so far)', moved, total;
        COMMIT;
        PERFORM pg_sleep(0.2);
    END LOOP;
    RAISE NOTICE 'done: % jobs re-keyed', total;
END
$$;
