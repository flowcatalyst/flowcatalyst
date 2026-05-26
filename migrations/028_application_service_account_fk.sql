-- 028_application_service_account_fk.sql
--
-- Adds a foreign key from `app_applications.service_account_id` to
-- `iam_principals.id` with `ON DELETE SET NULL`.
--
-- Why: the column was declared as plain VARCHAR(17) in migration 003
-- with no referential integrity. When the application's service
-- account was deleted, the `iam_principals` row went away but the
-- application's pointer at it lived on. The provision-service-account
-- handler then refused to mint a replacement because it saw a
-- non-null `service_account_id` and assumed one was already in place.
--
-- SET NULL (not CASCADE) is deliberate: the application itself should
-- survive its service account's deletion — we just want the dead
-- reference cleared so a new SA can be provisioned.

-- 1. Clean up existing dangling references (point at a principal that
--    no longer exists).
UPDATE app_applications
   SET service_account_id = NULL
 WHERE service_account_id IS NOT NULL
   AND service_account_id NOT IN (SELECT id FROM iam_principals);

-- 2. Drop if previously added (idempotent on a re-run).
ALTER TABLE app_applications
    DROP CONSTRAINT IF EXISTS app_applications_service_account_fk;

-- 3. Add the FK with ON DELETE SET NULL.
ALTER TABLE app_applications
    ADD CONSTRAINT app_applications_service_account_fk
    FOREIGN KEY (service_account_id)
    REFERENCES iam_principals (id)
    ON DELETE SET NULL;

-- Index for the cascade lookup (used when DELETE iam_principals fires
-- the SET NULL action — without this, PG full-scans the table).
CREATE INDEX IF NOT EXISTS idx_app_applications_service_account_id
    ON app_applications (service_account_id)
    WHERE service_account_id IS NOT NULL;
