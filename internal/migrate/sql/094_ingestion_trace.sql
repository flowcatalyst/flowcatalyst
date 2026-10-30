-- +goose Up
-- Ingestion traces (docs/ingestion-trace-design.md section 5): opt-in,
-- time-boxed capture per ingestion. Re-runnable: every statement is
-- IF [NOT] EXISTS or idempotent.
--
-- ing_trace_windows - one row per ingestion ever switched on; the window is
--                     live iff hard_stop > now(), compared when read, so
--                     expiry needs no job.
-- ing_trace_log     - append-only records shipped by the runtimes.
-- ing_exchange_metrics gains polls and last_poll_at, which every runtime
--                     reports whether or not tracing is on.
--
-- Cost (docs/large-table-schema-changes.md). The two new tables are empty, so
-- plain CREATE statements are safe, and neither has a foreign key (the
-- ingestion tables' convention, migration 088): the delete use case removes an
-- ingestion's window and records itself. ing_exchange_metrics is the large
-- one: polls has a constant default, so the ADD COLUMN is metadata only in
-- Postgres 11 and later (no rewrite, no backfill), and last_poll_at is
-- nullable. The ALTER takes a brief ACCESS EXCLUSIVE lock, so it fails fast
-- instead of queueing behind a long query and blocking every report.
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS ing_trace_windows (
    ingestion_id      TEXT        PRIMARY KEY,
    window_id         TEXT        NOT NULL,
    window_secs       INTEGER     NOT NULL,
    sample_every_secs INTEGER     NOT NULL,
    hard_stop         TIMESTAMPTZ NOT NULL,
    enabled_by        TEXT,
    enabled_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS ing_trace_log (
    id           TEXT           PRIMARY KEY,
    ingestion_id TEXT           NOT NULL,
    version      INTEGER        NOT NULL,
    window_id    TEXT           NOT NULL,
    capture_id   TEXT           NOT NULL,
    runtime_id   TEXT           NOT NULL,
    kind         TEXT           NOT NULL,
    level        TEXT           NOT NULL,
    message      TEXT           NOT NULL,
    data         JSONB,
    at           TIMESTAMPTZ(6) NOT NULL,
    created_at   TIMESTAMPTZ    NOT NULL
);

-- The page and the API read newest first by (at, id), one capture's records
-- by capture_id, and retention sweeps by the platform's own created_at.
CREATE INDEX IF NOT EXISTS idx_ing_trace_log_ingestion_at
    ON ing_trace_log (ingestion_id, at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_ing_trace_log_ingestion_capture
    ON ing_trace_log (ingestion_id, capture_id);
CREATE INDEX IF NOT EXISTS idx_ing_trace_log_created_at
    ON ing_trace_log (created_at);

ALTER TABLE ing_exchange_metrics ADD COLUMN IF NOT EXISTS polls INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ing_exchange_metrics ADD COLUMN IF NOT EXISTS last_poll_at TIMESTAMPTZ;

-- New permissions go to the roles that hold the matching ingestion
-- permissions, once (the seeder never rewrites an existing role):
--   platform:ingestion:trace:manage       <- every role holding ingestion:delete
--   platform:ingestion:trace:view-bodies  <- every role holding ingestion:delete
--   platform:ingestion:trace:view         <- every role holding ingestion:delete
--                                            or ingestion:update
-- delete is what the administrator holds and the operator does not, the
-- distinction the route author's permissions already lean on. update is what
-- the operator holds (it includes pause and resume). Neither the viewer nor
-- the pipeline's route author (platform:ingestion-route-author, which holds
-- update) gets any: a pipeline has no business reading captured bodies, and a
-- viewer reads none of them.
-- Tokens carry permissions from mint time: sign in again to pick them up.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT rp.role_id, p.permission
  FROM iam_role_permissions rp
  JOIN iam_roles r ON r.id = rp.role_id
 CROSS JOIN (VALUES
        ('platform:ingestion:trace:manage'),
        ('platform:ingestion:trace:view'),
        ('platform:ingestion:trace:view-bodies')
      ) AS p(permission)
 WHERE rp.permission = 'platform:ingestion:ingestion:delete'
   AND r.name <> 'platform:ingestion-route-author'
ON CONFLICT DO NOTHING;

INSERT INTO iam_role_permissions (role_id, permission)
SELECT rp.role_id, 'platform:ingestion:trace:view'
  FROM iam_role_permissions rp
  JOIN iam_roles r ON r.id = rp.role_id
 WHERE rp.permission = 'platform:ingestion:ingestion:update'
   AND r.name <> 'platform:ingestion-route-author'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission IN (
        'platform:ingestion:trace:view',
        'platform:ingestion:trace:manage',
        'platform:ingestion:trace:view-bodies');
ALTER TABLE ing_exchange_metrics DROP COLUMN IF EXISTS last_poll_at;
ALTER TABLE ing_exchange_metrics DROP COLUMN IF EXISTS polls;
DROP TABLE IF EXISTS ing_trace_log;
DROP TABLE IF EXISTS ing_trace_windows;
