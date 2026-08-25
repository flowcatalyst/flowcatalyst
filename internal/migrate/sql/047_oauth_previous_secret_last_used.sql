-- +goose Up
-- Observability for the secret-rotation overlap (migration 046).
--
-- The grace window tells you how long the old secret has left. It does not tell
-- you whether anyone is still using it, so "is it safe to revoke?" has no
-- answer until the window closes and the 401s arrive — a grace window without
-- this trades a loud failure for a silent one.
--
-- previous_secret_last_used_at is stamped whenever a client authenticates with
-- the superseded secret. Together with previous_secret_expires_at it answers the
-- whole question: how long is left, and is anyone still there.
--
-- Last-write-wins on a single timestamp: a fleet mid-rollout may authenticate
-- thousands of times an hour on the old secret, and the write is coalesced in
-- the application so a rollout does not become write load. NULL means the
-- superseded secret has not been used since the rotation — the reassuring state.
ALTER TABLE oauth_clients
    ADD COLUMN IF NOT EXISTS previous_secret_last_used_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE oauth_clients
    DROP COLUMN IF EXISTS previous_secret_last_used_at;
