-- +goose Up
-- Ingestion runtime hardening (docs/ingestion-design.md sections 3 and 7):
--
--   * ing_runtimes.builtin_kinds: the built-in source kinds a runtime declares
--     it carries (contract field builtinKinds); NULL is the v2 baseline set.
--   * ing_failures.category, dismissed_at, dismissed_by: a failure is a
--     "payload" failure (a record or payload the runtime could not take) or a
--     "rejected" notice (the platform refused the runtime's jobs request
--     permanently); every failure can be dismissed, and an undismissed
--     notice shows on the ingestion until someone does.
--   * ing_exchange_metrics.created_at: retention prunes by the time the
--     platform wrote a row, never by a timestamp the runtime supplied.
--   * ing_kind_requirements: the permission each runtime-reported kind last
--     declared, so it is enforced while no runtime that reports it is live.
--
-- Cost. Every table touched here was created by 088/089 and stays empty
-- unless FC_INGESTION_ENABLED was turned on; no installation runs ingestion
-- at volume yet. Even so each statement is the online form
-- (docs/large-table-schema-changes.md): the added columns are nullable or
-- have a constant default (no rewrite), the CHECK is added NOT VALID and then
-- validated, and the one backfill (metrics created_at from window_start)
-- touches only the rows the 30-day retention keeps. The plain CREATE INDEX
-- statements block writes to their table while they build: on an empty or
-- small table that is milliseconds. An installation that has run ingestion at
-- volume should build the two created_at indexes CONCURRENTLY by hand before
-- the deploy (the statements are IF NOT EXISTS, so this migration then skips
-- them).
SET LOCAL lock_timeout = '5s';

ALTER TABLE ing_runtimes ADD COLUMN IF NOT EXISTS builtin_kinds JSONB;

ALTER TABLE ing_failures ADD COLUMN IF NOT EXISTS category VARCHAR(20) NOT NULL DEFAULT 'payload';
ALTER TABLE ing_failures ADD COLUMN IF NOT EXISTS dismissed_at TIMESTAMPTZ;
ALTER TABLE ing_failures ADD COLUMN IF NOT EXISTS dismissed_by VARCHAR(17);
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_ing_failures_category') THEN
        ALTER TABLE ing_failures
            ADD CONSTRAINT chk_ing_failures_category CHECK (category IN ('payload', 'rejected')) NOT VALID;
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE ing_failures VALIDATE CONSTRAINT chk_ing_failures_category;
-- Retention sweeps by created_at (server-written) instead of occurred_at
-- (the runtime's clock), so the occurred_at index has no reader left.
CREATE INDEX IF NOT EXISTS idx_ing_failures_created ON ing_failures (created_at);
DROP INDEX IF EXISTS idx_ing_failures_occurred;
-- The notices the list and the drawer show: undismissed "rejected" failures.
CREATE INDEX IF NOT EXISTS idx_ing_failures_notices
    ON ing_failures (ingestion_id, id DESC) WHERE category = 'rejected' AND dismissed_at IS NULL;

ALTER TABLE ing_exchange_metrics ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ;
UPDATE ing_exchange_metrics SET created_at = window_start WHERE created_at IS NULL;
-- A row written by a platform that predates this column (a rolling deploy)
-- still gets a server time. Setting a default on an existing column does not
-- rewrite the table.
ALTER TABLE ing_exchange_metrics ALTER COLUMN created_at SET DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_ing_exchange_metrics_created ON ing_exchange_metrics (created_at);
DROP INDEX IF EXISTS idx_ing_exchange_metrics_window;

-- Route states of runtimes that are gone are pruned by runtime.
CREATE INDEX IF NOT EXISTS idx_ing_route_states_runtime ON ing_route_states (runtime_id);

-- The permission a runtime-reported kind declared in the last poll that
-- reported it ('' for none). Small: one row per kind ever reported.
CREATE TABLE IF NOT EXISTS ing_kind_requirements (
    kind                VARCHAR(50)  PRIMARY KEY,
    required_permission VARCHAR(200) NOT NULL DEFAULT '',
    runtime_id          VARCHAR(200) NOT NULL,
    updated_at          TIMESTAMPTZ  NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS ing_kind_requirements;
DROP INDEX IF EXISTS idx_ing_route_states_runtime;
CREATE INDEX IF NOT EXISTS idx_ing_exchange_metrics_window ON ing_exchange_metrics (window_start);
DROP INDEX IF EXISTS idx_ing_exchange_metrics_created;
ALTER TABLE ing_exchange_metrics DROP COLUMN IF EXISTS created_at;
CREATE INDEX IF NOT EXISTS idx_ing_failures_occurred ON ing_failures (occurred_at);
DROP INDEX IF EXISTS idx_ing_failures_notices;
DROP INDEX IF EXISTS idx_ing_failures_created;
ALTER TABLE ing_failures DROP CONSTRAINT IF EXISTS chk_ing_failures_category;
ALTER TABLE ing_failures DROP COLUMN IF EXISTS dismissed_by;
ALTER TABLE ing_failures DROP COLUMN IF EXISTS dismissed_at;
ALTER TABLE ing_failures DROP COLUMN IF EXISTS category;
ALTER TABLE ing_runtimes DROP COLUMN IF EXISTS builtin_kinds;
