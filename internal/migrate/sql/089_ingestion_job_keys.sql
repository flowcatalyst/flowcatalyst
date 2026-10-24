-- +goose Up
-- The idempotency keys of the jobs a runtime creates for an ingestion
-- (docs/ingestion-design.md section 7). A record carries an external id; the
-- first time one is seen for an ingestion its row is written in the same
-- transaction as the job, and a later request with the same key answers the
-- job it made, whatever its created_at. msg_dispatch_jobs cannot hold this
-- constraint (it is partitioned by created_at and carries no unique index on
-- external_id), and an index added to it for this would be paid for by every
-- other writer.
--
-- New and empty, so a plain CREATE is safe on a production database
-- (docs/large-table-schema-changes.md). High volume, so no foreign key (the
-- convention of ing_failures): a deleted ingestion's rows are left for
-- retention, which sweeps by created_at.
CREATE TABLE IF NOT EXISTS ing_job_keys (
    ingestion_id VARCHAR(17)  NOT NULL,
    external_id  VARCHAR(100) NOT NULL,
    job_id       VARCHAR(17)  NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL,
    PRIMARY KEY (ingestion_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_ing_job_keys_created
    ON ing_job_keys (created_at);

-- +goose Down
DROP TABLE IF EXISTS ing_job_keys;
