-- +goose Up
-- Drop the foreign keys whose child table is transactional (owner ruling
-- 2026-11-10, docs/foreign-keys.md): foreign keys stay between master and
-- configuration tables only. A table written per request or per message, or
-- one that retention prunes, carries none, so it can be partitioned or pruned
-- freely and an insert pays for no parent lookup.
--
-- Each of these was ON DELETE CASCADE. The delete paths now remove the
-- children themselves, in the same transaction as the parent:
--
--   iam_mfa_email_pins, iam_mfa_trusted_devices,
--   iam_reset_approval_requests      principal.Repository.Delete
--   ing_leases, ing_route_states,
--   ing_secret_fetches               ingestion.Repository.Delete
--   field_sign_in_codes              fieldidentity.IdentityRepo.Delete
--
-- except push_ingest_ids: a deleted application's push ids are left for the
-- push retention sweep (they are de-duplication keys under the application's
-- own id, which is never reused).
--
-- Columns and indexes stay; only the constraints go. Cost: catalogue only (no
-- scan, no rewrite). Dropping a foreign key takes a brief ACCESS EXCLUSIVE
-- lock on the child table and a SHARE ROW EXCLUSIVE lock on the parent (its
-- triggers are removed with it); lock_timeout makes the migration fail fast
-- rather than queue behind a long query (rerun it at a quiet moment).
-- Re-runnable: every statement is IF EXISTS.
SET LOCAL lock_timeout = '5s';

ALTER TABLE iam_mfa_email_pins
    DROP CONSTRAINT IF EXISTS iam_mfa_email_pins_principal_id_fkey;
ALTER TABLE iam_mfa_trusted_devices
    DROP CONSTRAINT IF EXISTS iam_mfa_trusted_devices_principal_id_fkey;
ALTER TABLE iam_reset_approval_requests
    DROP CONSTRAINT IF EXISTS iam_reset_approval_requests_principal_id_fkey;
ALTER TABLE ing_leases
    DROP CONSTRAINT IF EXISTS ing_leases_ingestion_id_fkey;
ALTER TABLE ing_route_states
    DROP CONSTRAINT IF EXISTS ing_route_states_ingestion_id_fkey;
ALTER TABLE ing_secret_fetches
    DROP CONSTRAINT IF EXISTS ing_secret_fetches_ingestion_id_fkey;
ALTER TABLE field_sign_in_codes
    DROP CONSTRAINT IF EXISTS field_sign_in_codes_identity_id_fkey;
ALTER TABLE push_ingest_ids
    DROP CONSTRAINT IF EXISTS push_ingest_ids_application_id_fkey;

-- +goose Down
-- Re-adds them NOT VALID: rows written while they were gone (orphans the
-- delete paths missed) are not checked, new rows are.
SET LOCAL lock_timeout = '5s';

ALTER TABLE iam_mfa_email_pins DROP CONSTRAINT IF EXISTS iam_mfa_email_pins_principal_id_fkey;
ALTER TABLE iam_mfa_email_pins ADD CONSTRAINT iam_mfa_email_pins_principal_id_fkey
    FOREIGN KEY (principal_id) REFERENCES iam_principals (id) ON DELETE CASCADE NOT VALID;
ALTER TABLE iam_mfa_trusted_devices DROP CONSTRAINT IF EXISTS iam_mfa_trusted_devices_principal_id_fkey;
ALTER TABLE iam_mfa_trusted_devices ADD CONSTRAINT iam_mfa_trusted_devices_principal_id_fkey
    FOREIGN KEY (principal_id) REFERENCES iam_principals (id) ON DELETE CASCADE NOT VALID;
ALTER TABLE iam_reset_approval_requests DROP CONSTRAINT IF EXISTS iam_reset_approval_requests_principal_id_fkey;
ALTER TABLE iam_reset_approval_requests ADD CONSTRAINT iam_reset_approval_requests_principal_id_fkey
    FOREIGN KEY (principal_id) REFERENCES iam_principals (id) ON DELETE CASCADE NOT VALID;
ALTER TABLE ing_leases DROP CONSTRAINT IF EXISTS ing_leases_ingestion_id_fkey;
ALTER TABLE ing_leases ADD CONSTRAINT ing_leases_ingestion_id_fkey
    FOREIGN KEY (ingestion_id) REFERENCES ing_ingestions (id) ON DELETE CASCADE NOT VALID;
ALTER TABLE ing_route_states DROP CONSTRAINT IF EXISTS ing_route_states_ingestion_id_fkey;
ALTER TABLE ing_route_states ADD CONSTRAINT ing_route_states_ingestion_id_fkey
    FOREIGN KEY (ingestion_id) REFERENCES ing_ingestions (id) ON DELETE CASCADE NOT VALID;
ALTER TABLE ing_secret_fetches DROP CONSTRAINT IF EXISTS ing_secret_fetches_ingestion_id_fkey;
ALTER TABLE ing_secret_fetches ADD CONSTRAINT ing_secret_fetches_ingestion_id_fkey
    FOREIGN KEY (ingestion_id) REFERENCES ing_ingestions (id) ON DELETE CASCADE NOT VALID;
ALTER TABLE field_sign_in_codes DROP CONSTRAINT IF EXISTS field_sign_in_codes_identity_id_fkey;
ALTER TABLE field_sign_in_codes ADD CONSTRAINT field_sign_in_codes_identity_id_fkey
    FOREIGN KEY (identity_id) REFERENCES field_identities (id) ON DELETE CASCADE NOT VALID;
ALTER TABLE push_ingest_ids DROP CONSTRAINT IF EXISTS push_ingest_ids_application_id_fkey;
ALTER TABLE push_ingest_ids ADD CONSTRAINT push_ingest_ids_application_id_fkey
    FOREIGN KEY (application_id) REFERENCES app_applications (id) ON DELETE CASCADE NOT VALID;
