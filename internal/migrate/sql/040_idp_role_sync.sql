-- +goose Up
-- Role-sync configuration moves from the email-domain mapping to the identity
-- provider. The IDP's `roles` claim vocabulary is defined by the IDP's app
-- registration, so which platform roles it may confer (and whether it syncs at
-- all) is a property of the provider, not of each domain routed to it.
--
-- The old columns (tnt_email_domain_mappings.sync_roles_from_idp and the
-- tnt_email_domain_mapping_allowed_roles junction) are left in place, dead,
-- for one release as rollback insurance; a later migration drops them.

-- Whether logins through this IDP reconcile the user's IDP_SYNC-sourced
-- platform roles from the token's `roles` claim.
ALTER TABLE oauth_identity_providers
    ADD COLUMN sync_roles_from_idp BOOLEAN NOT NULL DEFAULT FALSE;

-- Role sync has always run unconditionally for OIDC logins (the old per-domain
-- flag was never consulted), so existing OIDC providers keep that behaviour.
-- New providers start opted out.
UPDATE oauth_identity_providers SET sync_roles_from_idp = TRUE WHERE type = 'OIDC';

-- Allow-list of platform roles the IDP may confer via role sync (role TSIDs).
-- Empty = no restriction.
CREATE TABLE IF NOT EXISTS oauth_identity_provider_allowed_roles (
    id SERIAL PRIMARY KEY,
    identity_provider_id VARCHAR(17) NOT NULL,
    role_id VARCHAR(17) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_oauth_idp_allowed_roles_idp
    ON oauth_identity_provider_allowed_roles (identity_provider_id);

-- Backfill: union of each IDP's mappings' allow-lists — except when any of the
-- IDP's mappings has an EMPTY allow-list. Empty means "no restriction", so a
-- union that included a restricted sibling would tighten a domain that was
-- unrestricted; those IDPs stay unrestricted (no rows).
INSERT INTO oauth_identity_provider_allowed_roles (identity_provider_id, role_id)
SELECT DISTINCT m.identity_provider_id, ar.role_id
FROM tnt_email_domain_mappings m
JOIN tnt_email_domain_mapping_allowed_roles ar
    ON ar.email_domain_mapping_id = m.id
WHERE m.identity_provider_id NOT IN (
    SELECT m2.identity_provider_id
    FROM tnt_email_domain_mappings m2
    WHERE NOT EXISTS (
        SELECT 1 FROM tnt_email_domain_mapping_allowed_roles ar2
        WHERE ar2.email_domain_mapping_id = m2.id
    )
);
