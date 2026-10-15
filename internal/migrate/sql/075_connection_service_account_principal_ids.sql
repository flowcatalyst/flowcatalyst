-- +goose Up
-- 075_connection_service_account_principal_ids.sql
--
-- SyncConnections used to store the application's service_account_id as the
-- connection's service_account_id. The application column holds the service
-- account's PRINCIPAL id (prn_..., iam_principals.id; migration 028), while a
-- connection's column is the service account's OWN id (sac_...,
-- iam_service_accounts.id) -- the id the delivery signer looks up. Every
-- connection a code-first sync created therefore named a service account that
-- does not exist, and its deliveries went out unsigned
-- (docs/platform-bug-fixes.md F17).
--
-- Re-point those rows at the service account behind the principal. A
-- principal id never equals a service account id, so rows already correct are
-- untouched; a principal with no service account behind it is left as it is
-- (the next sync refuses it loudly).
UPDATE msg_connections c
   SET service_account_id = p.service_account_id,
       updated_at = NOW()
  FROM iam_principals p
 WHERE c.service_account_id = p.id
   AND p.service_account_id IS NOT NULL;

-- +goose Down
-- Data fix: not reversible (the old value named no service account).
SELECT 1;
