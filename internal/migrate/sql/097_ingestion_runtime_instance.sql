-- +goose Up
-- Ingestion runtime robustness (docs/ingestion-design.md section 7):
--
--   * ing_runtimes.instance_id: the process a runtime id was last polled by
--     (the poll's optional instanceId, a random id per process start). A poll
--     from another instance while that one is live is refused
--     (409 RUNTIME_ID_IN_USE), so two processes configured with one runtime
--     id cannot both hold its leases. NULL: the last poll sent none.
--   * ing_runtimes.left_at: when the runtime said goodbye
--     (POST /api/ingestion-runtime/v1/leases/release with goodbye). A runtime
--     is live while it polled within one lease and has not left since its
--     last poll (left_at IS NULL OR left_at < last_seen_at).
--
-- Cost (docs/large-table-schema-changes.md): ing_runtimes holds one row per
-- runtime seen in the last week. Both columns are nullable with no default,
-- so neither ALTER rewrites the table; each takes a brief ACCESS EXCLUSIVE
-- lock, so it fails fast instead of queueing behind a poll.
SET LOCAL lock_timeout = '5s';

ALTER TABLE ing_runtimes ADD COLUMN IF NOT EXISTS instance_id VARCHAR(64);
ALTER TABLE ing_runtimes ADD COLUMN IF NOT EXISTS left_at TIMESTAMPTZ;

-- +goose Down
SET LOCAL lock_timeout = '5s';

ALTER TABLE ing_runtimes DROP COLUMN IF EXISTS left_at;
ALTER TABLE ing_runtimes DROP COLUMN IF EXISTS instance_id;
