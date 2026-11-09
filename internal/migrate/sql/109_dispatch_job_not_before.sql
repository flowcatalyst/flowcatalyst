-- +goose Up
-- A dispatch job may carry the earliest instant it may be delivered.
--
-- A subscription's delaySeconds was stored and never enforced. The event
-- fan-out now copies it onto each job it raises as not_before =
-- created_at + delaySeconds (and the subscription's maxAgeSeconds as
-- expires_at = created_at + maxAgeSeconds), so a later edit of the
-- subscription does not move jobs already raised. The delivery callback
-- (POST /api/dispatch/process) enforces both: a job reached before
-- not_before is handed back to the router with the time to come back, and a
-- job reached after expires_at is marked EXPIRED.
--
-- Cost: nullable, no default, so adding it is a catalogue change only (no
-- rewrite, no scan) on the partitioned parent and every partition. It takes
-- a brief ACCESS EXCLUSIVE lock on msg_dispatch_jobs; lock_timeout makes the
-- migration fail fast rather than queue behind a long query (rerun it at a
-- quiet moment). No index: the callback reads the job by primary key.
-- msg_dispatch_jobs_read is not changed: the grid does not show it.
-- Every existing job, and every job created by the SDK routes, functions or
-- ingestion, has none.
SET LOCAL lock_timeout = '5s';

ALTER TABLE msg_dispatch_jobs
    ADD COLUMN IF NOT EXISTS not_before TIMESTAMPTZ;

-- +goose Down
ALTER TABLE msg_dispatch_jobs
    DROP COLUMN IF EXISTS not_before;
