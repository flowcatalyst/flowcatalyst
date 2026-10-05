-- +goose Up
-- msg_dispatch_queue: one row for every dispatch job whose status is PENDING,
-- and none for any other job.
--
-- A small, unpartitioned table with an ordinary index, kept exact by explicit
-- application writes from the dispatch-job lifecycle (no triggers, no partial
-- indexes). The scheduler's claim will read it in a later step; nothing reads
-- it yet. The schema is identical in every FlowCatalyst implementation that
-- shares the database: same table, columns, types and index name.
--
-- No foreign key: the parent is partitioned and its partitions are dropped.
-- job_created_at + job_id address the partitioned job row. version is the
-- job's updated_at when this row was written. claimed_at is for the scheduler's
-- claim (later); NULL here.
CREATE TABLE IF NOT EXISTS msg_dispatch_queue (
    job_id           VARCHAR(13)  PRIMARY KEY,
    job_created_at   TIMESTAMPTZ  NOT NULL,
    message_group    VARCHAR(200),
    sequence         INTEGER      NOT NULL,
    scheduled_for    TIMESTAMPTZ,
    subscription_id  VARCHAR(17),
    dispatch_pool_id VARCHAR(17),
    client_id        VARCHAR(17),
    mode             VARCHAR(30)  NOT NULL,
    queue            VARCHAR(255),
    version          TIMESTAMPTZ  NOT NULL,
    claimed_at       TIMESTAMPTZ,
    enqueued_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dispatch_queue_order
    ON msg_dispatch_queue (message_group NULLS LAST, sequence, job_created_at, job_id);

-- Backfill: every job that is PENDING now. Re-running is a no-op.
INSERT INTO msg_dispatch_queue
    (job_id, job_created_at, message_group, sequence, scheduled_for, subscription_id,
     dispatch_pool_id, client_id, mode, queue, version)
SELECT id, created_at, message_group, sequence, scheduled_for, subscription_id,
       dispatch_pool_id, client_id, mode, queue, updated_at
  FROM msg_dispatch_jobs
 WHERE status = 'PENDING'
ON CONFLICT (job_id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS msg_dispatch_queue;
