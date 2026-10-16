-- +goose Up
-- A dispatch job may carry the connection it is delivered through.
--
-- Until now a job reached a connection only through its subscription
-- (msg_subscriptions.connection_id), so pausing a connection held the jobs of
-- that connection's subscriptions and nothing else. A job created by a
-- function's fn.dispatch has no subscription: the connection it declares is
-- its own, and pausing that connection (a system down for maintenance) must
-- hold it. The scheduler's claim therefore also excludes jobs whose
-- connection_id is a PAUSED connection; the paused set is cached, so the claim
-- stays a single-table scan (no join).
--
-- Nullable, no default: every existing job, and every job created by the event
-- fan-out or by the SDK routes, has none. No index: the claim reads it as a
-- filter on rows the status index already yields. Added on the partitioned
-- parent, so every partition gets it. msg_dispatch_jobs_read is not changed:
-- the grid does not show it.
ALTER TABLE msg_dispatch_jobs
    ADD COLUMN IF NOT EXISTS connection_id VARCHAR(17);

-- +goose Down
ALTER TABLE msg_dispatch_jobs
    DROP COLUMN IF EXISTS connection_id;
