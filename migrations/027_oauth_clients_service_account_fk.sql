-- 027_oauth_clients_service_account_fk.sql
--
-- Adds a foreign key from `oauth_clients.service_account_principal_id`
-- to `iam_principals.id` with `ON DELETE CASCADE`.
--
-- Why: the column was declared as plain VARCHAR(17) in migration 007
-- with no referential integrity. Deleting a service account removed
-- the iam_principals + iam_service_accounts rows but left the OAuth
-- client (and its encrypted secret + grant types + redirect URIs +
-- allowed origins) orphaned and still active. The application-detail
-- UI counted that orphan as a live service account, and any attempt
-- to authenticate with the leftover client_id would have failed at a
-- much deeper layer.
--
-- Three steps so the migration is idempotent and survives prior
-- partial fixes:
--
--   1. Clean up existing orphans (rows pointing at a principal that
--      no longer exists). Cascade handles their junction children.
--   2. Drop the constraint if it already exists (re-run safe).
--   3. Add the FK with ON DELETE CASCADE.

-- 1. Delete orphaned oauth_clients whose service_account_principal_id
--    points at a missing principal. Junction tables (redirect_uris,
--    allowed_origins, grant_types, application_ids) cascade from
--    oauth_clients.id so they go with the parent.
DELETE FROM oauth_clients
WHERE service_account_principal_id IS NOT NULL
  AND service_account_principal_id NOT IN (SELECT id FROM iam_principals);

-- 2. Drop if previously added (idempotent on a re-run with a fix).
ALTER TABLE oauth_clients
    DROP CONSTRAINT IF EXISTS oauth_clients_service_account_fk;

-- 3. Add the FK with ON DELETE CASCADE.
ALTER TABLE oauth_clients
    ADD CONSTRAINT oauth_clients_service_account_fk
    FOREIGN KEY (service_account_principal_id)
    REFERENCES iam_principals (id)
    ON DELETE CASCADE;

-- Helpful for the cascade lookup and for any "list oauth clients
-- belonging to this principal" queries on the BFF side.
CREATE INDEX IF NOT EXISTS idx_oauth_clients_service_account_principal
    ON oauth_clients (service_account_principal_id)
    WHERE service_account_principal_id IS NOT NULL;
