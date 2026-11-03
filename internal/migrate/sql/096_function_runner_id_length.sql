-- +goose Up
-- A runner's id is the name it chooses for itself, not a platform TSID: the
-- ingestor runtime calls itself "runtime-<container hostname>", which on ECS
-- is about fifty characters, so every heartbeat it sent failed with
-- "value too long for type character varying(17)" and its JVM versions never
-- became READY. The heartbeat now refuses an id over 255 characters
-- (RUNNER_ID_TOO_LONG) instead of failing in the database.
--
-- Cost (docs/large-table-schema-changes.md): fng_runners holds one row per
-- runner. Raising a varchar limit is binary-coercible in Postgres, so neither
-- the table nor its primary-key index is rewritten. The ALTER takes a brief
-- ACCESS EXCLUSIVE lock, so it fails fast instead of queueing behind a
-- heartbeat.
SET LOCAL lock_timeout = '5s';

ALTER TABLE fng_runners ALTER COLUMN id TYPE VARCHAR(255);

-- +goose Down
SET LOCAL lock_timeout = '5s';

-- A runner re-registers on its next heartbeat, so its row can go.
DELETE FROM fng_runners WHERE length(id) > 17;
ALTER TABLE fng_runners ALTER COLUMN id TYPE VARCHAR(17);
