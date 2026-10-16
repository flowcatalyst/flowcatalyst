-- +goose Up
-- Re-publishing a digest whose version is FAILED resets it to PUBLISHED so a
-- runner retries it (docs/function-runner-plan.md §6.5). A runner that has
-- not yet seen the reset still reports its OLD FAILED outcome in heartbeats;
-- applying that stale report would immediately re-fail the version.
--
-- requeued_at_revision is the pool revision the reset published. A FAILED
-- report only counts when the runner has applied at least that revision
-- (heartbeat.revision >= requeued_at_revision), i.e. when it already holds
-- the post-reset desired state. 0 for every version that was never reset.
ALTER TABLE fng_versions ADD COLUMN IF NOT EXISTS requeued_at_revision BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE fng_versions DROP COLUMN IF EXISTS requeued_at_revision;
