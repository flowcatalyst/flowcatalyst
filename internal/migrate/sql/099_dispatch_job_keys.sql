-- +goose Up
-- The idempotency keys of dispatch jobs created through the SDK ingest routes
-- (POST /api/dispatch-jobs and /api/dispatch-jobs/batch, field
-- idempotencyKey). The first job to name a key under its scope wins it, and
-- its row is written in the same transaction as the job; a later request with
-- the same key answers that job's id instead of making another, whatever its
-- created_at.
--
-- The scope is the job's application (the first segment of its code, the
-- application facet the platform uses everywhere for a job) and its client
-- ('' for a job with none), so two tenants of one application that happen to
-- use the same key ("order-123") never answer each other's jobs.
--
-- msg_dispatch_jobs cannot hold this constraint: it is partitioned by
-- created_at, so a unique index must include created_at and would only catch
-- a repeat within one instant. The same reasoning as ing_job_keys (089).
--
-- New and empty, so a plain CREATE is safe on a production database
-- (docs/large-table-schema-changes.md). No foreign key: a job's partition is
-- dropped by retention and this table is swept on its own clock
-- (dispatchjob.JobKeysRetention, by created_at).
CREATE TABLE IF NOT EXISTS msg_dispatch_job_keys (
    application_code VARCHAR(100) NOT NULL,
    client_id        VARCHAR(17)  NOT NULL DEFAULT '',
    idempotency_key  VARCHAR(100) NOT NULL,
    job_id           VARCHAR(17)  NOT NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (application_code, client_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_msg_dispatch_job_keys_created
    ON msg_dispatch_job_keys (created_at);

-- +goose Down
DROP TABLE IF EXISTS msg_dispatch_job_keys;
