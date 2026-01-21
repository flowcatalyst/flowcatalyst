-- 0004: Create OIDC payloads table for oidc-provider
-- Stores all oidc-provider artifacts (authorization codes, tokens, sessions, etc.)
-- Uses a flexible JSONB payload structure as recommended by oidc-provider.

CREATE TABLE oidc_payloads (
    -- Primary identifier (model-specific ID from oidc-provider)
    id VARCHAR(128) PRIMARY KEY,

    -- Model type discriminator (Session, AccessToken, AuthorizationCode, RefreshToken, etc.)
    type VARCHAR(64) NOT NULL,

    -- The actual payload data (oidc-provider manages structure)
    payload JSONB NOT NULL,

    -- Grant ID - links related tokens (access, refresh) to same grant
    grant_id VARCHAR(128),

    -- User code - for device authorization flow
    user_code VARCHAR(128),

    -- UID - unique identifier for certain model types
    uid VARCHAR(128),

    -- Expiration timestamp
    expires_at TIMESTAMPTZ,

    -- Consumption timestamp (for single-use artifacts like auth codes)
    consumed_at TIMESTAMPTZ,

    -- Creation timestamp
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for grant-based lookups (revocation)
CREATE INDEX oidc_payloads_grant_id_idx ON oidc_payloads(grant_id) WHERE grant_id IS NOT NULL;

-- Index for user code lookups (device flow)
CREATE INDEX oidc_payloads_user_code_idx ON oidc_payloads(user_code) WHERE user_code IS NOT NULL;

-- Index for UID lookups
CREATE INDEX oidc_payloads_uid_idx ON oidc_payloads(uid) WHERE uid IS NOT NULL;

-- Index for type-based queries
CREATE INDEX oidc_payloads_type_idx ON oidc_payloads(type);

-- Index for expiration cleanup
CREATE INDEX oidc_payloads_expires_at_idx ON oidc_payloads(expires_at) WHERE expires_at IS NOT NULL;

-- Composite index for type + expiration (cleanup queries)
CREATE INDEX oidc_payloads_type_expires_idx ON oidc_payloads(type, expires_at) WHERE expires_at IS NOT NULL;
