-- +goose Up
-- A scheduled-job instance can now be deferred: a target that answers 429
-- with Retry-After is asked again no sooner than not_before, without the
-- attempt counting against delivery_max_attempts (dispatch jobs already
-- treat 429 this way). NULL means deliverable now, which is every existing
-- row. The dispatcher's queued-instance query filters on it.
ALTER TABLE msg_scheduled_job_instances ADD COLUMN IF NOT EXISTS not_before TIMESTAMPTZ;

-- +goose Down
ALTER TABLE msg_scheduled_job_instances DROP COLUMN IF EXISTS not_before;
