-- +goose Up
-- The instant before which a browser session (fc_session cookie) no longer counts.
--
-- The session cookie is a self-contained JWT with no server-side record, so
-- changing a password, resetting a password or 2FA, or deactivating a user
-- could not end a session someone had already captured. sessions_valid_after
-- is bumped to "now" by those operations; the cookie path rejects a session
-- whose issued-at (one-second resolution) is earlier. NULL = never bumped:
-- every session counts, exactly as before this column.
--
-- Cost: nullable, no default, so adding it is a catalogue change only (no
-- rewrite, no scan). It takes a brief ACCESS EXCLUSIVE lock on iam_principals;
-- lock_timeout makes the migration fail fast rather than queue behind a long
-- query. No index: the column is read with its row, by primary key.
SET LOCAL lock_timeout = '5s';

ALTER TABLE iam_principals
    ADD COLUMN IF NOT EXISTS sessions_valid_after TIMESTAMPTZ;

-- +goose Down
ALTER TABLE iam_principals
    DROP COLUMN IF EXISTS sessions_valid_after;
