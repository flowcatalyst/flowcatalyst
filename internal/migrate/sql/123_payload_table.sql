-- +goose Up
-- Payloads get a table of their own (docs/payload-offload-design.md,
-- sections 3 and 9). An event and every dispatch job point at a row of
-- msg_payloads; nothing else holds a payload.
--
-- msg_payloads: one row per stored payload, written once and never updated.
--   id          a TSID.
--   created_at  the partition key, and NOT the time the row was written: it
--               is the created_at of the event or job the payload was stored
--               for. So the payload is always read by primary key from the
--               row that points at it, (payload_id, created_at), and it is
--               dropped by retention in the same month as that row.
--   data        the payload as the text the producer sent. NULL when it was
--               offloaded.
--   ref         where the payload is in the payload store when it was
--               offloaded (payloadstore.Ref, opaque to SQL). NULL when it is
--               in `data`.
--   size        the payload's length in bytes as sent.
--   Exactly one of data and ref is set (the CHECK).
--
-- payload_id on msg_events, msg_events_read and msg_dispatch_jobs: the
-- payload of that row, in the partition of the row's own created_at. NULL on
-- a row with no payload, and on every row written before this migration,
-- whose payload is still where it was (msg_events.data,
-- msg_dispatch_jobs.payload); those columns are dropped by a later migration,
-- after one retention period (section 10).
--   - A job the fan-out raises takes its event's payload_id (and has its
--     event's created_at, so it is the same partition).
--   - A job created directly gets a payload row of its own.
--   - Editing a job's payload inserts a new payload row under the job's
--     created_at and points the job at it; the event and the other jobs keep
--     the original.
--
-- Partitions: monthly, named msg_payloads_YYYY_MM like the other messaging
-- tables. One is created here for every month msg_events or
-- msg_dispatch_jobs has a partition, with that partition's own bounds, so a
-- job of an earlier month can still be given an edited payload. From then on
-- the partition manager keeps them (add msg_payloads to
-- stream.PartitionedTables in the same change).
--
-- Cost: one new, empty table and its empty partitions. Three nullable
-- columns with no default, so a catalogue change only (no rewrite, no scan)
-- on the three partitioned parents and every partition; each ALTER takes a
-- brief ACCESS EXCLUSIVE lock, and lock_timeout makes the migration fail fast
-- rather than queue behind a long query (rerun at a quiet moment). No index
-- on payload_id: a payload is read from the row that points at it, never the
-- other way round.
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS msg_payloads (
    id         VARCHAR(13) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    data       TEXT,
    ref        TEXT,
    size       INTEGER     NOT NULL,
    PRIMARY KEY (id, created_at),
    CONSTRAINT chk_msg_payloads_data_or_ref CHECK ((data IS NULL) <> (ref IS NULL))
) PARTITION BY RANGE (created_at);

-- +goose StatementBegin
DO $$
DECLARE
    p RECORD;
BEGIN
    FOR p IN
        SELECT DISTINCT ON (suffix) suffix, bound
          FROM (
            SELECT right(c.relname, 7) AS suffix,
                   pg_get_expr(c.relpartbound, c.oid) AS bound
              FROM pg_inherits i
              JOIN pg_class c ON c.oid = i.inhrelid
              JOIN pg_class parent ON parent.oid = i.inhparent
             WHERE parent.relname IN ('msg_events', 'msg_dispatch_jobs')
               AND c.relname ~ '_[0-9]{4}_[0-9]{2}$'
          ) existing
         ORDER BY suffix
    LOOP
        EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF msg_payloads %s',
                       'msg_payloads_' || p.suffix, p.bound);
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE msg_events
    ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);

ALTER TABLE msg_events_read
    ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);

ALTER TABLE msg_dispatch_jobs
    ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);

-- +goose Down
ALTER TABLE msg_dispatch_jobs DROP COLUMN IF EXISTS payload_id;
ALTER TABLE msg_events_read   DROP COLUMN IF EXISTS payload_id;
ALTER TABLE msg_events        DROP COLUMN IF EXISTS payload_id;
DROP TABLE IF EXISTS msg_payloads;
