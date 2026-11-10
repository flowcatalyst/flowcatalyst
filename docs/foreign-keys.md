# Foreign keys

## Policy

Owner ruling, 2026-11-10:

> We can keep some foreign keys but not ones that ever need to be partitioned, and we should avoid
> them in future. Master data would be the best candidate for them. Transactional data should
> ideally never have foreign keys.

What that means in this schema:

- **Master and configuration data may keep its foreign keys.** These are low-volume tables that
  people edit: applications, roles, principals, clients, OAuth clients, connections, subscriptions,
  pools, function definitions, ingestion definitions, field-worker identities and their grants,
  push configuration. Both ends of the key must be master data.
- **Transactional data carries none.** A table is transactional when its rows are written per
  request or per message (a sign-in, a poll, a push, a delivery), when retention or an expiry sweep
  prunes it, or when it is, or could one day need to be, partitioned. Events, dispatch jobs and
  attempts, traces, logs, login attempts, sessions, tokens, challenges, leases, runtime reports,
  push ids, audit and outbox tables are all transactional.
- **Columns and indexes stay** when a constraint goes. Only the constraint is dropped.
- **A dropped `ON DELETE CASCADE` becomes code.** The parent's delete path removes the children in
  the same transaction, or the doc below says why the orphans are harmless (pruned by retention).

This replaces the 2026-08-27 plan to remove every foreign key (`docs/foreign-key-removal-plan.md`,
now a pointer here). The master-data keys stay, including the two cross-aggregate ones that plan
worried about (`oauth_clients_service_account_fk`, `app_applications_service_account_fk`).

## Rule for new tables

1. **Do not add a foreign key by default.** Reference other tables by a plain column with an index.
2. A foreign key is allowed only when **both** tables are master or configuration data and the key
   earns its place (a junction owned by its parent, cleaned up by cascade). Never on a table that is
   written per request or per message, pruned, or partitioned.
3. If the table holds references without a key, the parent's delete path deletes or keeps the
   children explicitly, with an integration test that deletes a parent with children present and
   counts what is left.
4. `internal/migrate/foreign_keys_pg_test.go` lists every foreign key the schema may hold and
   fails on any other. Adding one means adding it there and to the table below, in the same change.

## The audit

Every `REFERENCES` in `internal/migrate/sql`, checked against a fully migrated database (all 40
existed before migration 114; none had been dropped by a later migration). Constraint names are the
real ones (`pg_constraint`).

### Kept: master and configuration data (32)

| Child table | Column | Parent | On delete | Migration | Constraint | Why it is master data |
|---|---|---|---|---|---|---|
| `iam_principal_roles` | `principal_id` | `iam_principals` | CASCADE | 002 | `iam_principal_roles_principal_id_fkey` | role assignments, edited by people |
| `iam_role_permissions` | `role_id` | `iam_roles` | CASCADE | 002 | `iam_role_permissions_role_id_fkey` | role definition |
| `oauth_client_redirect_uris` | `oauth_client_id` | `oauth_clients` | CASCADE | 007 | `oauth_client_redirect_uris_oauth_client_id_fkey` | OAuth client configuration |
| `oauth_client_allowed_origins` | `oauth_client_id` | `oauth_clients` | CASCADE | 007 | `oauth_client_allowed_origins_oauth_client_id_fkey` | OAuth client configuration |
| `oauth_client_grant_types` | `oauth_client_id` | `oauth_clients` | CASCADE | 007 | `oauth_client_grant_types_oauth_client_id_fkey` | OAuth client configuration |
| `oauth_client_application_ids` | `oauth_client_id` | `oauth_clients` | CASCADE | 007 | `oauth_client_application_ids_oauth_client_id_fkey` | OAuth client configuration |
| `oauth_client_post_logout_redirect_uris` | `oauth_client_id` | `oauth_clients` | CASCADE | 029 | `fk_oauth_client_post_logout_redirect_uris_client` | OAuth client configuration |
| `oauth_clients` | `service_account_principal_id` | `iam_principals` | CASCADE | 027 | `oauth_clients_service_account_fk` | both ends master data |
| `app_applications` | `service_account_id` | `iam_principals` | SET NULL | 028 | `app_applications_service_account_fk` | both ends master data |
| `webauthn_credentials` | `principal_id` | `iam_principals` | CASCADE | 020 | `webauthn_credentials_principal_id_fkey` | a user's registered passkeys; few, not pruned |
| `iam_user_mfa_methods` | `principal_id` | `iam_principals` | CASCADE | 031 | `iam_user_mfa_methods_principal_id_fkey` | a user's enrolled factors |
| `iam_user_mfa_recovery_codes` | `principal_id` | `iam_principals` | CASCADE | 031 | `iam_user_mfa_recovery_codes_principal_id_fkey` | a fixed set per user, replaced on regeneration, not pruned |
| `app_application_openapi_specs` | `application_id` | `app_applications` | CASCADE | 025 | `app_application_openapi_specs_application_id_fkey` | one row per synced spec version |
| `portal_identity_apps` | `identity_id` | `portal_identities` | CASCADE | 053 | `portal_identity_apps_identity_id_fkey` | grants |
| `portal_identity_apps` | `portal_app_id` | `portal_apps` | CASCADE | 053 | `portal_identity_apps_portal_app_id_fkey` | grants |
| `fng_versions` | `function_id` | `fng_functions` | CASCADE | 059 | `fng_versions_function_id_fkey` | function definitions (pruned to a kept count, still definitions) |
| `fng_aliases` | `function_id` | `fng_functions` | CASCADE | 059 | `fng_aliases_function_id_fkey` | function configuration |
| `fng_settings` | `function_id` | `fng_functions` | CASCADE | 059 | `fng_settings_function_id_fkey` | function configuration |
| `fng_routes` | `function_id` | `fng_functions` | CASCADE | 062 | `fng_routes_function_id_fkey` | function configuration |
| `ing_ingestions` | `application_id` | `app_applications` | NO ACTION | 088 | `ing_ingestions_application_id_fkey` | ingestion definition; blocks deleting an application that has ingestions |
| `ing_ingestions` | `client_id` | `tnt_clients` | NO ACTION | 088 | `ing_ingestions_client_id_fkey` | ingestion definition; blocks deleting a client that has ingestions |
| `ing_ingestions` | `managed_by` | `ing_sets` | SET NULL | 088 | `ing_ingestions_managed_by_fkey` | ingestion definition |
| `ing_versions` | `ingestion_id` | `ing_ingestions` | CASCADE | 088 | `ing_versions_ingestion_id_fkey` | saved definitions |
| `ing_set_applies` | `set_id` | `ing_sets` | CASCADE | 088 | `ing_set_applies_set_id_fkey` | apply history, one row per apply, kept to the newest N per set |
| `ing_state` | `ingestion_id` | `ing_ingestions` | CASCADE | 088 | `ing_state_ingestion_id_fkey` | **ambiguous, kept**: see below |
| `field_identity_apps` | `identity_id` | `field_identities` | CASCADE | 095 | `field_identity_apps_identity_id_fkey` | grants |
| `field_device_apps` | `device_id` | `field_devices` | CASCADE | 095 | `field_device_apps_device_id_fkey` | grants |
| `field_badges` | `identity_id` | `field_identities` | CASCADE | 101 | `field_badges_identity_id_fkey` | printed badges, issued by people, one active per worker |
| `push_rules` | `application_id` | `app_applications` | CASCADE | 104 | `push_rules_application_id_fkey` | synced push configuration |
| `push_ticket_keys` | `application_id` | `app_applications` | CASCADE | 104 | `push_ticket_keys_application_id_fkey` | synced push configuration |
| `push_app_settings` | `application_id` | `app_applications` | CASCADE | 104 | `push_app_settings_application_id_fkey` | per-application limits, set by people |
| `push_trace_windows` | `application_id` | `app_applications` | CASCADE | 108 | `push_trace_windows_application_id_fkey` | one row per application, switched by an operator, not pruned |

### Dropped: transactional data (8), migration 114

All eight were `ON DELETE CASCADE`.

| Child table | Column | Parent | Constraint | Why it is transactional | Children now handled by |
|---|---|---|---|---|---|
| `iam_mfa_email_pins` | `principal_id` | `iam_principals` | `iam_mfa_email_pins_principal_id_fkey` | one row per sign-in challenge, expires in minutes | `principal.Repository.Delete` |
| `iam_mfa_trusted_devices` | `principal_id` | `iam_principals` | `iam_mfa_trusted_devices_principal_id_fkey` | one row per remembered sign-in (a token), expires | `principal.Repository.Delete` |
| `iam_reset_approval_requests` | `principal_id` | `iam_principals` | `iam_reset_approval_requests_principal_id_fkey` | one row per reset request, expires | `principal.Repository.Delete` |
| `ing_leases` | `ingestion_id` | `ing_ingestions` | `ing_leases_ingestion_id_fkey` | written on every runtime poll, pruned | `ingestion.Repository.Delete`; retention also prunes expired leases of ingestions that are not live (gone included) |
| `ing_route_states` | `ingestion_id` | `ing_ingestions` | `ing_route_states_ingestion_id_fkey` | written on every runtime report, pruned | `ingestion.Repository.Delete`; retention also removes rows of ingestions that no longer exist |
| `ing_secret_fetches` | `ingestion_id` | `ing_ingestions` | `ing_secret_fetches_ingestion_id_fkey` | upserted on every secret fetch, pruned | `ingestion.Repository.Delete`; retention also removes rows of ingestions that no longer exist |
| `field_sign_in_codes` | `identity_id` | `field_identities` | `field_sign_in_codes_identity_id_fkey` | single-use 10-minute codes, pruned | `fieldidentity.IdentityRepo.Delete` |
| `push_ingest_ids` | `application_id` | `app_applications` | `push_ingest_ids_application_id_fkey` | one row per accepted push, pruned past push retention | left for the push retention sweep (`pushgw.PruneIngestIDs`): de-duplication keys under the deleted application's id, which is never reused, and nothing reads them once the application is gone |

Every explicit cleanup has an integration test that deletes a parent with children present and
counts the rows left, for that parent and for a neighbour:
`internal/platform/principal/foreign_key_cleanup_pg_test.go`,
`internal/platform/ingestion/foreign_key_cleanup_pg_test.go`,
`internal/platform/fieldidentity/foreign_key_cleanup_pg_test.go`.

Notes on the delete paths:

- SERVICE principals are also deleted by `serviceaccount.Repository` (by `service_account_id`). They
  never sign in interactively, so they never have email PINs, remembered devices or reset requests;
  nothing is needed there.
- Without the key, a write that races a parent's delete can leave an orphan the delete did not see.
  For the ingestion rows the retention sweep removes them; sign-in codes expire and are swept
  (`fieldidentity.SessionRepo.Purge`); push ids are pruned. Email PINs and remembered devices
  expire, but `mfa.Repository.PurgeExpiredEmailPins` / `PurgeExpiredTrustedDevices` exist and
  nothing schedules them today, and reset-approval requests have no sweep at all (an open item from
  the docs review). An orphan of those three names a principal that no longer exists, so no sign-in
  or approval can use it; it is dead weight until a sweep is scheduled.

### Ambiguous, kept for an owner decision

- **`ing_state`** (`ing_state_ingestion_id_fkey`): a polled source's watermark and cursor values.
  Written by the runtime that holds the lease, on its polls (transactional by how it is written),
  but bounded to a few keys per ingestion, never pruned, and part of the ingestion's own durable
  state (master-like by what it is). Kept. Dropping it would need `ingestion.Repository.Delete` to
  delete it explicitly (one line).

## Migration 114

`internal/migrate/sql/114_drop_transactional_foreign_keys.sql` drops the eight with
`DROP CONSTRAINT IF EXISTS`, under `lock_timeout = 5s` (catalogue-only; a brief ACCESS EXCLUSIVE on
each child and SHARE ROW EXCLUSIVE on each parent). It has no effect probe (it only drops, and is
idempotent). Its Down re-adds them `NOT VALID`, so a rollback succeeds over orphans written in the
meantime and checks only new rows.
