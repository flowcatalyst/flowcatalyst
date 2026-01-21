-- V6: Flatten JSONB columns and change events.data to TEXT

-- Part 1: events.data JSONB → TEXT
-- Event payloads aren't guaranteed to be JSON, so store as TEXT
ALTER TABLE events ALTER COLUMN data TYPE TEXT;

-- Part 2: Flatten principals.user_identity
ALTER TABLE principals ADD COLUMN email VARCHAR(255);
ALTER TABLE principals ADD COLUMN email_domain VARCHAR(100);
ALTER TABLE principals ADD COLUMN idp_type VARCHAR(50);
ALTER TABLE principals ADD COLUMN external_idp_id VARCHAR(255);
ALTER TABLE principals ADD COLUMN password_hash VARCHAR(255);
ALTER TABLE principals ADD COLUMN last_login_at TIMESTAMPTZ;

UPDATE principals
SET email = user_identity->>'email',
    email_domain = user_identity->>'emailDomain',
    idp_type = user_identity->>'idpType',
    external_idp_id = user_identity->>'externalIdpId',
    password_hash = user_identity->>'passwordHash',
    last_login_at = CASE
        WHEN user_identity->>'lastLoginAt' IS NULL THEN NULL
        -- Handle Unix epoch format (numeric string like "1768565525.708297000")
        WHEN user_identity->>'lastLoginAt' ~ '^\d+\.?\d*$'
        THEN to_timestamp((user_identity->>'lastLoginAt')::double precision)
        -- Handle ISO date format
        ELSE (user_identity->>'lastLoginAt')::timestamptz
    END
WHERE user_identity IS NOT NULL;

DROP INDEX IF EXISTS idx_principals_email;
CREATE UNIQUE INDEX idx_principals_email ON principals(email) WHERE email IS NOT NULL;
CREATE INDEX idx_principals_email_domain ON principals(email_domain) WHERE email_domain IS NOT NULL;
ALTER TABLE principals DROP COLUMN user_identity;

-- Part 3: Flatten service_accounts.webhook_credentials
ALTER TABLE service_accounts ADD COLUMN wh_auth_type VARCHAR(50) DEFAULT 'BEARER_TOKEN';
ALTER TABLE service_accounts ADD COLUMN wh_auth_token_ref VARCHAR(500);
ALTER TABLE service_accounts ADD COLUMN wh_signing_secret_ref VARCHAR(500);
ALTER TABLE service_accounts ADD COLUMN wh_signing_algorithm VARCHAR(50) DEFAULT 'HMAC_SHA256';
ALTER TABLE service_accounts ADD COLUMN wh_credentials_created_at TIMESTAMPTZ;
ALTER TABLE service_accounts ADD COLUMN wh_credentials_regenerated_at TIMESTAMPTZ;

UPDATE service_accounts
SET wh_auth_type = COALESCE(webhook_credentials->>'authType', 'BEARER_TOKEN'),
    wh_auth_token_ref = webhook_credentials->>'authTokenRef',
    wh_signing_secret_ref = webhook_credentials->>'signingSecretRef',
    wh_signing_algorithm = COALESCE(webhook_credentials->>'signingAlgorithm', 'HMAC_SHA256'),
    wh_credentials_created_at = CASE
        WHEN webhook_credentials->>'createdAt' IS NULL THEN NULL
        WHEN webhook_credentials->>'createdAt' ~ '^\d+\.?\d*$'
        THEN to_timestamp((webhook_credentials->>'createdAt')::double precision)
        ELSE (webhook_credentials->>'createdAt')::timestamptz
    END,
    wh_credentials_regenerated_at = CASE
        WHEN webhook_credentials->>'regeneratedAt' IS NULL THEN NULL
        WHEN webhook_credentials->>'regeneratedAt' ~ '^\d+\.?\d*$'
        THEN to_timestamp((webhook_credentials->>'regeneratedAt')::double precision)
        ELSE (webhook_credentials->>'regeneratedAt')::timestamptz
    END
WHERE webhook_credentials IS NOT NULL;

ALTER TABLE service_accounts DROP COLUMN webhook_credentials;
