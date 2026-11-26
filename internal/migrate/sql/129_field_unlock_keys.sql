-- +goose Up
-- Field-worker unlock keys (docs/field-unlock-keys-spec.md section 5).
--
-- field_unlock_keys holds the public half of a key pair a worker's PERSONAL
-- phone made in its secure hardware, usable only after a strong biometric
-- match. The platform verifies the key's signature over a fresh challenge and
-- issues a session (amr hwk, uv) without a PIN. One LIVE key per device (the
-- partial unique index): a phone's biometrics cannot tell its people apart, so
-- a second worker's key would let the first worker's finger sign in as them.
-- Revoked rows are kept (the console's history) until the purger removes them
-- 30 days after revocation. attestation_chain is the certificate chain as the
-- phone sent it (public data); attestation is the platform's parsed summary,
-- and attestation_verified whether it met the policy's REQUIRED rule when the
-- key was registered. pin_verified_at is the last time the worker proved their
-- PIN on the device; the policy's pin_reauth_days counts from it.
--
-- field_unlock_challenges holds the single-use challenges an unlock signs,
-- bound to one key on one device, as their SHA-256.
--
-- field_policies.unlock_keys_enabled (default false) is the client's switch;
-- unlock_key_attestation (NONE, the default, or REQUIRED) whether a key must
-- come with a trusted Android key attestation.
--
-- No foreign keys (docs/foreign-keys.md): deleting a worker or a device
-- deletes its keys and challenges in its own transaction.
--
-- Cost (docs/large-table-schema-changes.md does not apply): two new small
-- tables, at most one live row per device; two columns with constant defaults
-- on field_policies (one row per client that has a policy); a nullable column
-- with no default on field_sessions (metadata only) and a partial index on it
-- that is empty when built (no session has an unlock key yet; field_sessions is
-- purged 30 days after its rows end).
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS field_unlock_keys (
    id                   VARCHAR(17)  PRIMARY KEY,
    client_id            VARCHAR(17)  NOT NULL,
    identity_id          VARCHAR(17)  NOT NULL,
    device_id            VARCHAR(17)  NOT NULL,
    jkt                  VARCHAR(64)  NOT NULL,
    public_key_jwk       TEXT         NOT NULL,
    attestation_chain    TEXT,
    attestation          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    attestation_verified BOOLEAN      NOT NULL DEFAULT FALSE,
    pin_verified_at      TIMESTAMPTZ  NOT NULL,
    created_at           TIMESTAMPTZ  NOT NULL,
    last_used_at         TIMESTAMPTZ,
    revoked_at           TIMESTAMPTZ,
    revoked_reason       VARCHAR(30)  CHECK (revoked_reason IN ('REPLACED', 'PIN_CHANGED', 'PIN_RESET',
                             'WORKER_SUSPENDED', 'WORKER_LOCKED', 'DEVICE_REVOKED', 'POLICY_DISABLED',
                             'POLICY_CHANGED', 'ADMIN', 'DEVICE', 'ATTESTATION_REVOKED')),
    revoked_by           VARCHAR(100)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_field_unlock_keys_jkt ON field_unlock_keys (jkt);
CREATE UNIQUE INDEX IF NOT EXISTS uq_field_unlock_keys_live_device
    ON field_unlock_keys (device_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_field_unlock_keys_identity ON field_unlock_keys (identity_id);
CREATE INDEX IF NOT EXISTS idx_field_unlock_keys_live_client
    ON field_unlock_keys (client_id) WHERE revoked_at IS NULL;

-- field_sessions.unlock_key_id names the unlock key that signed a login family
-- in (NULL for every other sign-in), so revoking the key can end the sessions it
-- started (docs/field-unlock-keys-spec.md section 7).
ALTER TABLE field_sessions ADD COLUMN IF NOT EXISTS unlock_key_id VARCHAR(17);
CREATE INDEX IF NOT EXISTS idx_field_sessions_unlock_key
    ON field_sessions (unlock_key_id) WHERE unlock_key_id IS NOT NULL AND revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS field_unlock_challenges (
    challenge_hash BYTEA        PRIMARY KEY,
    unlock_key_id  VARCHAR(17)  NOT NULL,
    device_id      VARCHAR(17)  NOT NULL,
    expires_at     TIMESTAMPTZ  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_field_unlock_challenges_expires ON field_unlock_challenges (expires_at);

ALTER TABLE field_policies ADD COLUMN IF NOT EXISTS unlock_keys_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE field_policies ADD COLUMN IF NOT EXISTS unlock_key_attestation VARCHAR(10) NOT NULL DEFAULT 'NONE'
    CHECK (unlock_key_attestation IN ('NONE', 'REQUIRED'));
-- pin_reauth_days (migration 095, default 7, read by nothing until now) becomes
-- the unlock key's periodic PIN, default 0 = never (the owner, 2026-11-26).
-- Stored rows keep their value: a client's own choice cannot be told from the
-- old default, so it is left to the client (docs/field-unlock-keys-spec.md 4.4).
ALTER TABLE field_policies ALTER COLUMN pin_reauth_days SET DEFAULT 0;

-- +goose Down
SET LOCAL lock_timeout = '5s';
ALTER TABLE field_policies ALTER COLUMN pin_reauth_days SET DEFAULT 7;
DROP INDEX IF EXISTS idx_field_sessions_unlock_key;
ALTER TABLE field_sessions DROP COLUMN IF EXISTS unlock_key_id;
ALTER TABLE field_policies DROP COLUMN IF EXISTS unlock_key_attestation;
ALTER TABLE field_policies DROP COLUMN IF EXISTS unlock_keys_enabled;
DROP TABLE IF EXISTS field_unlock_challenges;
DROP TABLE IF EXISTS field_unlock_keys;
