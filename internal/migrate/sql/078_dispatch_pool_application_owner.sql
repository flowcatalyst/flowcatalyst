-- +goose Up
-- 078_dispatch_pool_application_owner.sql
--
-- A dispatch pool created by an application's SDK sync records the
-- application that owns it (docs/platform-bug-fixes.md F19). Pools are global
-- rows matched by code; the built-in platform:application-service role now
-- carries platform:messaging:dispatch-pool:sync, so without an owner any
-- application's service account could overwrite another application's pool or
-- a platform pool by sending its code. The sync lets an application-scoped
-- caller change only pools whose application_id is its own application.
--
-- NULL = no owner (every pool that exists today, and pools created in the
-- admin UI/API): only an all-applications caller may sync those, and doing so
-- hands them to the application being synced. No foreign key (see
-- docs/foreign-key-removal-plan.md).
ALTER TABLE msg_dispatch_pools ADD COLUMN IF NOT EXISTS application_id VARCHAR(17);
CREATE INDEX IF NOT EXISTS idx_msg_dispatch_pools_application_id ON msg_dispatch_pools (application_id);

-- +goose Down
DROP INDEX IF EXISTS idx_msg_dispatch_pools_application_id;
ALTER TABLE msg_dispatch_pools DROP COLUMN IF EXISTS application_id;
