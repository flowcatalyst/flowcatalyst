-- V35: Add allowed_role_ids junction table for email_domain_mappings
--
-- This table stores the allowed role IDs for each email domain mapping.
-- When users authenticate from a domain with allowed roles configured,
-- only roles in this list will be synced/allowed for the user.

CREATE TABLE IF NOT EXISTS email_domain_mapping_allowed_roles (
    email_domain_mapping_id VARCHAR(17) NOT NULL REFERENCES email_domain_mappings(id) ON DELETE CASCADE,
    role_id VARCHAR(17) NOT NULL,
    PRIMARY KEY (email_domain_mapping_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_edm_allowed_roles_mapping
    ON email_domain_mapping_allowed_roles(email_domain_mapping_id);

COMMENT ON TABLE email_domain_mapping_allowed_roles IS
    'Junction table for allowed roles per email domain mapping. If populated, users from this domain can only have these roles.';
