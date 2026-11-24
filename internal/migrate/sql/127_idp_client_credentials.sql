-- +goose Up
-- How an OIDC identity provider proves the platform's identity at the token
-- endpoint (docs/oidc-client-credentials-spec.md). Until now the only proof was
-- a client secret, which an external provider such as Entra expires after at
-- most 24 months; the code exchange then fails (AADSTS7000222). Two more
-- credentials, both RFC 7523 private_key_jwt:
--
--   * oauth_identity_providers.oidc_client_auth_method: CLIENT_SECRET (today's
--     behaviour, the default, so every existing row keeps working), CERTIFICATE
--     (a client assertion signed by the provider's ACTIVE certificate key) or
--     FEDERATED (a client assertion signed by the platform signing key).
--   * oauth_identity_provider_certificates: the platform-generated key pairs
--     for CERTIFICATE. The private key is stored encrypted with the app key
--     (PKCS#8 PEM, EncryptBound, aad = table || ":" || id); the certificate is
--     public. At most one ACTIVE and one NEXT row per provider.
--
-- No foreign key on the certificates (docs/foreign-keys.md: new tables get
-- none by default): DeleteIdentityProvider removes a provider's certificates in
-- the same transaction, and an integration test counts what is left.
--
-- Cost (docs/large-table-schema-changes.md does not apply): both tables are
-- small configuration tables (a handful of identity providers). The new column
-- has a constant default, so the ALTER does not rewrite the table; it takes a
-- brief ACCESS EXCLUSIVE lock, so it fails fast instead of queueing.
SET LOCAL lock_timeout = '5s';

ALTER TABLE oauth_identity_providers
    ADD COLUMN IF NOT EXISTS oidc_client_auth_method VARCHAR(30) NOT NULL DEFAULT 'CLIENT_SECRET';

CREATE TABLE IF NOT EXISTS oauth_identity_provider_certificates (
    id                   VARCHAR(17) PRIMARY KEY,
    identity_provider_id VARCHAR(17) NOT NULL,
    status               VARCHAR(10) NOT NULL,
    private_key_ref      TEXT NOT NULL,
    certificate_pem      TEXT NOT NULL,
    thumbprint_sha1      VARCHAR(40) NOT NULL,
    thumbprint_sha256    VARCHAR(64) NOT NULL,
    not_before           TIMESTAMPTZ NOT NULL,
    not_after            TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_idp_certificates_status UNIQUE (identity_provider_id, status)
);

-- +goose Down
DROP TABLE IF EXISTS oauth_identity_provider_certificates;

SET LOCAL lock_timeout = '5s';

ALTER TABLE oauth_identity_providers DROP COLUMN IF EXISTS oidc_client_auth_method;
