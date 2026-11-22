-- +goose Up
-- A runtime id belongs to the account that first registered it (owner's
-- decision D1, 2026-11-22; docs/ingestion-design.md section 7, "The runtime
-- id's owner"):
--
--   * ing_runtimes.principal_id: the principal of the service account whose
--     poll registered the runtime id. Set by the first poll and never changed
--     by a later one; a call naming the id from another account is refused
--     (403 RUNTIME_ID_NOT_OWNED) on every runtime route. NULL: the row was
--     written before this migration; the next poll binds it to its caller.
--     The binding ends with the row: retention removes a runtime unseen for
--     7 days, and the id is then free to be registered again.
--
-- No foreign key (the ingestion tables' convention, docs/foreign-keys.md).
--
-- Cost (docs/large-table-schema-changes.md): ing_runtimes holds one row per
-- runtime seen in the last week. The column is nullable with no default, so
-- the ALTER does not rewrite the table; it takes a brief ACCESS EXCLUSIVE
-- lock, so it fails fast instead of queueing behind a poll.
SET LOCAL lock_timeout = '5s';

ALTER TABLE ing_runtimes ADD COLUMN IF NOT EXISTS principal_id VARCHAR(17);

-- +goose Down
SET LOCAL lock_timeout = '5s';

ALTER TABLE ing_runtimes DROP COLUMN IF EXISTS principal_id;
