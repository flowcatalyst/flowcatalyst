ALTER TABLE email_domain_mappings
    ADD COLUMN sync_roles_from_idp BOOLEAN NOT NULL DEFAULT FALSE;
