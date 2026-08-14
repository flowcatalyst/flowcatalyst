-- +goose Up
-- Portal identity plane (docs/portal-identity-plan.md Phase 2.5 v2): portal
-- end-users are a SEPARATE identity population, one row per (client, email)
-- context — wholly unrelated to iam_principals. The platform implements the
-- plane (password machinery, reset tokens, OIDC bridge reuse) but the portal
-- login endpoints are independent of the employee auth surface and never
-- touch fc_session.
CREATE TABLE IF NOT EXISTS portal_identities (
    id VARCHAR(17) PRIMARY KEY,
    client_id VARCHAR(17) NOT NULL,               -- portal-operator tenant client
    email VARCHAR(255) NOT NULL,                  -- stored lowercased
    name VARCHAR(255),
    password_hash VARCHAR(255),                   -- NULL until the invite completes (or SSO-only)
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE', -- ACTIVE | DISABLED
    source VARCHAR(20) NOT NULL,                  -- INVITE | JIT
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_portal_identities_client_email UNIQUE (client_id, email)
);

CREATE INDEX IF NOT EXISTS idx_portal_identities_email
    ON portal_identities (email);

-- Short-lived single-use stash for the portal authorization flow: GET
-- /portal/authorize validates the (portal-flagged) OAuth client + redirect
-- URI + PKCE, parks the chain here, and bounces to the SPA portal login
-- page; the password/SSO handlers redeem the row to mint the code. TTL is
-- enforced on redeem; a janitor may purge expired rows.
CREATE TABLE IF NOT EXISTS portal_login_flows (
    id VARCHAR(64) PRIMARY KEY,
    oauth_client_id VARCHAR(100) NOT NULL,        -- oauth_clients.client_id
    portal_client_id VARCHAR(17) NOT NULL,        -- owner tenant client (denormalized at stash time)
    redirect_uri VARCHAR(2000) NOT NULL,
    scope VARCHAR(500),
    state VARCHAR(500) NOT NULL,
    nonce VARCHAR(500),
    code_challenge VARCHAR(200),
    code_challenge_method VARCHAR(10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- Non-NULL marks an OAuth client as a portal entry point owned by that
-- tenant client: it routes through /portal/authorize and its codes carry
-- portal-identity subjects. Ordinary first-party OAuth clients (NULL) are
-- untouched.
ALTER TABLE oauth_clients
    ADD COLUMN IF NOT EXISTS portal_client_id VARCHAR(17);

-- Binds an identity provider to a client's portal plane: portal flows may
-- only route to IdPs whose portal_client_id matches the flow's client.
-- NULL = an employee-plane IdP (email-domain-mapping routing), unavailable
-- to portals.
ALTER TABLE oauth_identity_providers
    ADD COLUMN IF NOT EXISTS portal_client_id VARCHAR(17);

-- Marks an OIDC login state as a PORTAL-plane handshake for the given owner
-- client: the bridge callback then routes into the portal sink (JIT portal
-- identity + code issuance, no fc_session) instead of the employee flow.
ALTER TABLE oauth_oidc_login_states
    ADD COLUMN IF NOT EXISTS portal_client_id VARCHAR(17);

-- Post-set-password redirect for portal invites: validated against the
-- registered redirect URIs of the owning client's portal OAuth clients when
-- the token is minted, then followed by the SPA after a successful confirm.
-- (Reset tokens key portal identities by their ptu_ id in principal_id — the
-- two id spaces share the column but never collide thanks to TSID prefixes.)
ALTER TABLE iam_password_reset_tokens
    ADD COLUMN IF NOT EXISTS redirect_uri VARCHAR(2000);

-- +goose Down
ALTER TABLE iam_password_reset_tokens DROP COLUMN IF EXISTS redirect_uri;
ALTER TABLE oauth_oidc_login_states DROP COLUMN IF EXISTS portal_client_id;
ALTER TABLE oauth_identity_providers DROP COLUMN IF EXISTS portal_client_id;
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS portal_client_id;
DROP TABLE IF EXISTS portal_login_flows;
DROP INDEX IF EXISTS idx_portal_identities_email;
DROP TABLE IF EXISTS portal_identities;
