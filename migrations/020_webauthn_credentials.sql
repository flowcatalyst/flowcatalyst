-- FlowCatalyst WebAuthn Credentials Table
-- Stores public-key credentials registered by internal-auth users.
-- Federated users never have rows here (gated at application layer by
-- email_domain_mapping presence — see crates/fc-platform/src/webauthn).
--
-- All fields are non-secret by spec: the private key never leaves the
-- authenticator (Secure Enclave / TPM / YubiKey). No application-layer
-- encryption is applied. See project_passkeys_scope.md.
--
-- The full webauthn-rs Passkey is stored as JSONB in `passkey_data`. We
-- denormalise `credential_id` for indexed lookup at authentication time.
-- Counter / backup-state updates are applied by webauthn-rs and the whole
-- blob is rewritten on each successful authentication.

CREATE TABLE IF NOT EXISTS webauthn_credentials (
    id              VARCHAR(17) PRIMARY KEY,
    principal_id    VARCHAR(17) NOT NULL REFERENCES iam_principals(id) ON DELETE CASCADE,
    credential_id   BYTEA NOT NULL UNIQUE,
    passkey_data    JSONB NOT NULL,
    name            VARCHAR(120),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_principal ON webauthn_credentials (principal_id);
