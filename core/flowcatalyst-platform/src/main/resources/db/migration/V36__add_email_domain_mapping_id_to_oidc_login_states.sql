-- V36: Add email_domain_mapping_id column to oidc_login_states
--
-- The OIDC login state now tracks the email domain mapping ID
-- to properly associate the login with the correct mapping configuration.

ALTER TABLE oidc_login_states
ADD COLUMN IF NOT EXISTS email_domain_mapping_id VARCHAR(17);

COMMENT ON COLUMN oidc_login_states.email_domain_mapping_id IS
    'Reference to the email domain mapping used for this login session';
