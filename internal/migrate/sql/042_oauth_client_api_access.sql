-- +goose Up
-- Per-client opt-in restoring authority-bearing access tokens for
-- interactive logins. The identity/api token split stays the DEFAULT
-- (authorization_code mints identity-only tokens); a trusted first-party
-- OAuth client flagged api_access instead mints token_use=api access tokens
-- whose roles (and derived scope/applications) are narrowed to the client's
-- own application_ids. Mutually exclusive with portal_client_id — portal
-- identities never carry authority.
ALTER TABLE oauth_clients
    ADD COLUMN IF NOT EXISTS api_access BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE oauth_clients DROP COLUMN IF EXISTS api_access;
