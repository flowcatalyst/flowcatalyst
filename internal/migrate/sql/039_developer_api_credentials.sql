-- +goose Up
-- Self-service developer API credentials: a regular User principal holding
-- the `developer` role can mint client_credentials tokens as themselves
-- (client_id = their own principal id) using a dedicated, rotatable secret
-- stored here — never their login password. Additive + nullable, mirrors
-- password_hash sitting directly on this table for other user-identity
-- material (1:1 per principal, no side table needed).

ALTER TABLE iam_principals ADD COLUMN dev_client_secret_ref TEXT;
ALTER TABLE iam_principals ADD COLUMN dev_client_secret_updated_at TIMESTAMPTZ;
