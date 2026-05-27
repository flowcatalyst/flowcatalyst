-- +goose Up
-- FlowCatalyst — post-logout redirect URI whitelist
--
-- Per OIDC RP-Initiated Logout 1.0 §2: "If a post_logout_redirect_uri parameter
-- value is supplied, the OP MUST verify the supplied URI is in the list
-- registered for that Client." Prior to this migration the logout endpoint did
-- only an HTTPS + suspicious-pattern heuristic, which is weaker than a
-- registered whitelist and allowed open-redirect-style abuse against any
-- attacker-owned HTTPS subdomain.
--
-- Mirrors the shape of `oauth_client_redirect_uris` (junction table, cascade
-- delete) so the same hydrate / save / persist code patterns apply.

CREATE TABLE IF NOT EXISTS oauth_client_post_logout_redirect_uris (
    oauth_client_id           VARCHAR(17) NOT NULL,
    post_logout_redirect_uri  TEXT        NOT NULL,
    PRIMARY KEY (oauth_client_id, post_logout_redirect_uri),
    CONSTRAINT fk_oauth_client_post_logout_redirect_uris_client
        FOREIGN KEY (oauth_client_id)
        REFERENCES oauth_clients(id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_oauth_client_post_logout_redirect_uris_client
    ON oauth_client_post_logout_redirect_uris (oauth_client_id);
