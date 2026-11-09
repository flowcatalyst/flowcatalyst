-- +goose Up
-- Scheduled jobs: exactly-once-per-tick guarantee, stuck-IN_FLIGHT recovery, and
-- normalising legacy cron expressions.
--
-- 1. msg_scheduled_job_ticks: one row per (job, scheduled tick). The cron poller
--    claims a tick by inserting here (ON CONFLICT DO NOTHING) in the same
--    transaction as the instance row, so two pollers (split brain, failover
--    overlap) can never both fire the same tick. This cannot be a unique index on
--    msg_scheduled_job_instances: that table is RANGE-partitioned by created_at
--    (022) and a partitioned unique index must include the partition key.
--    Rows are pruned by the poller after 30 days (created_at index).
CREATE TABLE IF NOT EXISTS msg_scheduled_job_ticks (
    scheduled_job_id VARCHAR(17)  NOT NULL,
    scheduled_for    TIMESTAMPTZ  NOT NULL,
    instance_id      VARCHAR(17)  NOT NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (scheduled_job_id, scheduled_for)
);
CREATE INDEX IF NOT EXISTS idx_msg_scheduled_job_ticks_created
    ON msg_scheduled_job_ticks (created_at);

-- 2. in_flight_at: when the dispatcher last marked the instance IN_FLIGHT, so an
--    instance orphaned by a crash can be recognised (older than the HTTP timeout
--    plus a margin) and abandoned without being re-fired. NULL on pre-existing
--    rows; recovery falls back to fired_at.
ALTER TABLE msg_scheduled_job_instances ADD COLUMN IF NOT EXISTS in_flight_at TIMESTAMPTZ;

-- 3. Cron expressions fire only in the 6-field seconds-first form. Earlier
--    releases accepted 5-field POSIX expressions at save time but they never
--    fired. Prefix a '0' seconds field to every stored 5-field expression, which
--    is exactly the standard-cron meaning ("at second 0 of each matching
--    minute"). Such a job has never run, so an ACTIVE one is PAUSED (owner
--    ruling 2026-11-09): nothing starts firing unexpectedly; an operator resumes
--    the ones that are wanted. 7-field expressions (with a year) have no
--    equivalent and are left as-is: the poller logs a warning for any stored
--    expression it cannot parse.
UPDATE msg_scheduled_jobs j
   SET crons = ARRAY(
         SELECT CASE WHEN cardinality(regexp_split_to_array(btrim(u.c), '\s+')) = 5
                     THEN '0 ' || btrim(u.c) ELSE u.c END
           FROM unnest(j.crons) WITH ORDINALITY AS u(c, ord)
          ORDER BY u.ord),
       status = CASE WHEN j.status = 'ACTIVE' THEN 'PAUSED' ELSE j.status END
 WHERE EXISTS (SELECT 1 FROM unnest(j.crons) AS x(c)
                WHERE cardinality(regexp_split_to_array(btrim(x.c), '\s+')) = 5);

-- +goose Down
DROP TABLE IF EXISTS msg_scheduled_job_ticks;
ALTER TABLE msg_scheduled_job_instances DROP COLUMN IF EXISTS in_flight_at;
