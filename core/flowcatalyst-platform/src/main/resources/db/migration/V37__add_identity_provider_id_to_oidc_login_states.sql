-- V37: Add identity_provider_id column to oidc_login_states
--
-- The OIDC login state tracks the identity provider ID
-- to properly associate the login with the correct IDP configuration.

ALTER TABLE oidc_login_states
ADD COLUMN IF NOT EXISTS identity_provider_id VARCHAR(17);

COMMENT ON COLUMN oidc_login_states.identity_provider_id IS
    'Reference to the identity provider used for this login session';
