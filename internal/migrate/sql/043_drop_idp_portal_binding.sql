-- +goose Up
-- The per-IdP portal binding is removed (owner decision 2026-08-20): an
-- identity provider is an AUTHENTICATOR for the domains it owns — any login
-- surface (employee plane or any client's portal) routes owned domains to
-- it. Which client's portal identity a portal login yields comes from the
-- flow's OAuth client (oauth_clients.portal_client_id), which stays. The
-- binding wrongly limited one IdP to one portal, breaking the anchor
-- scenario of a shared org IdP serving several portals.
ALTER TABLE oauth_identity_providers DROP COLUMN IF EXISTS portal_client_id;

-- +goose Down
ALTER TABLE oauth_identity_providers ADD COLUMN IF NOT EXISTS portal_client_id VARCHAR(17);
