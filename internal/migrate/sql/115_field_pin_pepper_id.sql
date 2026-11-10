-- +goose Up
-- Which pepper a field-worker PIN hash was made under.
--
-- The PIN pepper derives from FLOWCATALYST_APP_KEY, so rotating the key
-- changes the pepper. pin_pepper_id is a short fingerprint of the pepper (not
-- of the key) written with every new PEPPER_V1 hash; verification picks the
-- matching pepper (current or one derived from FLOWCATALYST_APP_KEY_PREVIOUS)
-- by it, so a wrong PIN costs one argon2id however many keys are configured.
-- NULL for every hash stored before this column, and for LEGACY hashes (no
-- pepper): those are tried under the current pepper, then each previous one,
-- and re-hashed with an id on the worker's next sign-in.
--
-- Cost: nullable, no default, so adding it is a catalogue change only (no
-- rewrite, no scan). It takes a brief ACCESS EXCLUSIVE lock on
-- field_identities; lock_timeout makes the migration fail fast rather than
-- queue behind a long query. No index: the column is read with its row.
SET LOCAL lock_timeout = '5s';

ALTER TABLE field_identities
    ADD COLUMN IF NOT EXISTS pin_pepper_id VARCHAR(16);

-- +goose Down
ALTER TABLE field_identities
    DROP COLUMN IF EXISTS pin_pepper_id;
