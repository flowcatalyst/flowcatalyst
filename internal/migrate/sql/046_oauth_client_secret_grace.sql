-- +goose Up
-- Overlap window for OAuth client secret rotation.
--
-- Rotation used to be a hard cutover: SetSecretRef overwrote
-- client_secret_ref, and verification decrypted exactly that one ref with no
-- fallback, so the instant rotate returned every caller still presenting the
-- old secret got 401. For CONFIDENTIAL clients — machine-to-machine services
-- holding the secret in config or a secrets manager — that makes rotation a
-- synchronised operation: no window where both secrets work, so a fleet can't
-- be rolled gradually and a rollback to the previous deployment resurrects a
-- dead secret. The practical result is that people rotate rarely, which is the
-- opposite of the point.
--
-- previous_secret_ref holds the immediately-prior encrypted secret and
-- previous_secret_expires_at bounds how long it stays acceptable.
-- Verification tries the current ref first, then the previous one while it is
-- unexpired. Both columns are cleared when the overlap is ended early
-- (revoke-previous-secret) and by the periodic purger once lapsed, so a dead
-- secret is not retained at rest.
--
-- Additive and nullable: existing rows read as "no overlap in flight", which is
-- exactly the old behaviour.
ALTER TABLE oauth_clients
    ADD COLUMN IF NOT EXISTS previous_secret_ref TEXT,
    ADD COLUMN IF NOT EXISTS previous_secret_expires_at TIMESTAMPTZ;

-- Lets the purger walk in-flight overlaps by expiry instead of scanning every
-- client. Partial: only rows actually carrying a previous secret qualify.
CREATE INDEX IF NOT EXISTS idx_oauth_clients_previous_secret_expires_at
    ON oauth_clients (previous_secret_expires_at)
    WHERE previous_secret_ref IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_oauth_clients_previous_secret_expires_at;
ALTER TABLE oauth_clients
    DROP COLUMN IF EXISTS previous_secret_ref,
    DROP COLUMN IF EXISTS previous_secret_expires_at;
