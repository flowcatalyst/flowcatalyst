-- +goose Up
-- Ingestion drafts and secret fetches (owner rulings 2026-10-25,
-- docs/ingestion-design.md sections 3, 4 and 7):
--
--   * ing_versions.definition_digest: the sha256 (hex) of the version's
--     definition in its canonical form, so a save that says what an existing
--     version says reuses that version instead of making a duplicate (an
--     update without promote saves a draft, deduplicated by digest). The
--     canonical form is Postgres's own jsonb text (keys sorted, duplicates
--     dropped, no insignificant whitespace, numbers normalised): the digest
--     is computed by the database, at insert and at lookup, with the same
--     expression as the backfill below, so there is one canonicalisation.
--   * ing_secret_fetches: when each runtime last fetched each secret an
--     ingestion binds, and how often. A runtime fetching its ingestion's
--     bound secrets is security telemetry (a log line and a counter), not an
--     audited config reveal; this row is what the pages and the API show of
--     it. Upserted on every fetch; a runtime's rows are pruned once it is
--     gone (unseen for 7 days), with its route states.
--
-- Cost. ing_versions was created by 088 and grows by one row per saved
-- version (a few per ingestion; no installation runs ingestion at volume
-- yet). The added column is nullable (no rewrite); the backfill touches
-- every version row once, which is small; the index is built plainly, which
-- blocks writes to ing_versions for the milliseconds a small table takes. An
-- installation with an unusually large ing_versions should build
-- idx_ing_versions_digest CONCURRENTLY by hand before the deploy (the
-- statement is IF NOT EXISTS). ing_secret_fetches is new and empty.
SET LOCAL lock_timeout = '5s';

ALTER TABLE ing_versions ADD COLUMN IF NOT EXISTS definition_digest VARCHAR(64);
UPDATE ing_versions
SET definition_digest = encode(sha256(convert_to(definition::text, 'UTF8')), 'hex')
WHERE definition_digest IS NULL;
CREATE INDEX IF NOT EXISTS idx_ing_versions_digest ON ing_versions (ingestion_id, definition_digest);

CREATE TABLE IF NOT EXISTS ing_secret_fetches (
    ingestion_id    VARCHAR(17)  NOT NULL REFERENCES ing_ingestions(id) ON DELETE CASCADE,
    secret_name     VARCHAR(201) NOT NULL,
    runtime_id      VARCHAR(200) NOT NULL,
    -- The live version the fetch was made under, and who made it.
    version         INTEGER      NOT NULL,
    principal_id    VARCHAR(17),
    last_fetched_at TIMESTAMPTZ  NOT NULL,
    fetch_count     BIGINT       NOT NULL DEFAULT 1,
    CONSTRAINT pk_ing_secret_fetches PRIMARY KEY (ingestion_id, secret_name, runtime_id)
);
-- The secret fetches of runtimes that are gone are pruned by runtime.
CREATE INDEX IF NOT EXISTS idx_ing_secret_fetches_runtime ON ing_secret_fetches (runtime_id);

-- +goose Down
DROP TABLE IF EXISTS ing_secret_fetches;
DROP INDEX IF EXISTS idx_ing_versions_digest;
ALTER TABLE ing_versions DROP COLUMN IF EXISTS definition_digest;
