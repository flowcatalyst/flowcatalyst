-- V7: Add 3-character prefixes to all typed IDs
-- Format: {prefix}_{tsid} (e.g., "clt_0HZXEQ5Y8JY5Z")
-- Total length: 17 characters (3 prefix + 1 underscore + 13 TSID)
--
-- This follows the Stripe pattern where typed IDs are stored WITH the prefix
-- in the database, eliminating serialization/deserialization overhead.
--
-- NOTE: We use position('_' in col) = 0 instead of NOT LIKE '%_%' because
-- underscore is a wildcard in SQL LIKE patterns.

-- =============================================================================
-- Part 1: Expand VARCHAR columns from 13 to 17 characters
-- =============================================================================

-- Core tables - primary keys
ALTER TABLE clients ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE principals ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE applications ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE service_accounts ALTER COLUMN id TYPE VARCHAR(17);

-- Authorization tables
ALTER TABLE auth_roles ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE auth_permissions ALTER COLUMN id TYPE VARCHAR(17);

-- Event/Messaging tables
ALTER TABLE event_types ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE events ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE events_read ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE subscriptions ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE dispatch_pools ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs_read ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE schemas ALTER COLUMN id TYPE VARCHAR(17);

-- Authentication tables
ALTER TABLE oauth_clients ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE authorization_codes ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE idp_role_mappings ALTER COLUMN id TYPE VARCHAR(17);

-- Configuration tables
ALTER TABLE client_auth_configs ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE application_client_configs ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE client_access_grants ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE anchor_domains ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE cors_allowed_origins ALTER COLUMN id TYPE VARCHAR(17);

-- Audit tables
ALTER TABLE audit_logs ALTER COLUMN id TYPE VARCHAR(17);

-- =============================================================================
-- Part 2: Expand VARCHAR foreign key columns from 13 to 17 characters
-- =============================================================================

-- principals foreign keys
ALTER TABLE principals ALTER COLUMN client_id TYPE VARCHAR(17);
ALTER TABLE principals ALTER COLUMN application_id TYPE VARCHAR(17);

-- applications foreign keys
ALTER TABLE applications ALTER COLUMN service_account_id TYPE VARCHAR(17);

-- service_accounts foreign keys
ALTER TABLE service_accounts ALTER COLUMN application_id TYPE VARCHAR(17);

-- auth_roles foreign keys
ALTER TABLE auth_roles ALTER COLUMN application_id TYPE VARCHAR(17);

-- auth_permissions foreign keys
ALTER TABLE auth_permissions ALTER COLUMN application_id TYPE VARCHAR(17);

-- events foreign keys
ALTER TABLE events ALTER COLUMN client_id TYPE VARCHAR(17);

-- events_read foreign keys (event_id removed - id IS the event id)
ALTER TABLE events_read ALTER COLUMN client_id TYPE VARCHAR(17);

-- subscriptions foreign keys
ALTER TABLE subscriptions ALTER COLUMN client_id TYPE VARCHAR(17);
ALTER TABLE subscriptions ALTER COLUMN dispatch_pool_id TYPE VARCHAR(17);
ALTER TABLE subscriptions ALTER COLUMN service_account_id TYPE VARCHAR(17);

-- dispatch_pools foreign keys
ALTER TABLE dispatch_pools ALTER COLUMN client_id TYPE VARCHAR(17);

-- dispatch_jobs foreign keys
ALTER TABLE dispatch_jobs ALTER COLUMN event_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs ALTER COLUMN service_account_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs ALTER COLUMN client_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs ALTER COLUMN subscription_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs ALTER COLUMN dispatch_pool_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs ALTER COLUMN schema_id TYPE VARCHAR(17);

-- dispatch_jobs_read foreign keys
ALTER TABLE dispatch_jobs_read ALTER COLUMN event_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs_read ALTER COLUMN service_account_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs_read ALTER COLUMN client_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs_read ALTER COLUMN subscription_id TYPE VARCHAR(17);
ALTER TABLE dispatch_jobs_read ALTER COLUMN dispatch_pool_id TYPE VARCHAR(17);

-- schemas foreign keys
ALTER TABLE schemas ALTER COLUMN event_type_id TYPE VARCHAR(17);

-- oauth_clients foreign keys
ALTER TABLE oauth_clients ALTER COLUMN service_account_principal_id TYPE VARCHAR(17);

-- authorization_codes foreign keys
ALTER TABLE authorization_codes ALTER COLUMN principal_id TYPE VARCHAR(17);
ALTER TABLE authorization_codes ALTER COLUMN context_client_id TYPE VARCHAR(17);

-- refresh_tokens foreign keys
ALTER TABLE refresh_tokens ALTER COLUMN principal_id TYPE VARCHAR(17);
ALTER TABLE refresh_tokens ALTER COLUMN context_client_id TYPE VARCHAR(17);

-- oidc_login_states foreign keys
ALTER TABLE oidc_login_states ALTER COLUMN auth_config_id TYPE VARCHAR(17);

-- client_auth_configs foreign keys
ALTER TABLE client_auth_configs ALTER COLUMN client_id TYPE VARCHAR(17);
ALTER TABLE client_auth_configs ALTER COLUMN primary_client_id TYPE VARCHAR(17);

-- application_client_configs foreign keys
ALTER TABLE application_client_configs ALTER COLUMN application_id TYPE VARCHAR(17);
ALTER TABLE application_client_configs ALTER COLUMN client_id TYPE VARCHAR(17);

-- client_access_grants foreign keys
ALTER TABLE client_access_grants ALTER COLUMN principal_id TYPE VARCHAR(17);
ALTER TABLE client_access_grants ALTER COLUMN client_id TYPE VARCHAR(17);

-- cors_allowed_origins foreign keys
ALTER TABLE cors_allowed_origins ALTER COLUMN created_by TYPE VARCHAR(17);

-- audit_logs foreign keys
ALTER TABLE audit_logs ALTER COLUMN entity_id TYPE VARCHAR(17);
ALTER TABLE audit_logs ALTER COLUMN principal_id TYPE VARCHAR(17);

-- =============================================================================
-- Part 3: Add prefixes to existing IDs (primary keys first)
-- =============================================================================

-- clients (clt_)
UPDATE clients SET id = 'clt_' || id WHERE position('_' in id) = 0;

-- principals (prn_)
UPDATE principals SET id = 'prn_' || id WHERE position('_' in id) = 0;

-- applications (app_)
UPDATE applications SET id = 'app_' || id WHERE position('_' in id) = 0;

-- service_accounts (sac_)
UPDATE service_accounts SET id = 'sac_' || id WHERE position('_' in id) = 0;

-- auth_roles (rol_)
UPDATE auth_roles SET id = 'rol_' || id WHERE position('_' in id) = 0;

-- auth_permissions (prm_)
UPDATE auth_permissions SET id = 'prm_' || id WHERE position('_' in id) = 0;

-- event_types (evt_)
UPDATE event_types SET id = 'evt_' || id WHERE position('_' in id) = 0;

-- events (evn_)
UPDATE events SET id = 'evn_' || id WHERE position('_' in id) = 0;

-- events_read (evr_)
UPDATE events_read SET id = 'evr_' || id WHERE position('_' in id) = 0;

-- subscriptions (sub_)
UPDATE subscriptions SET id = 'sub_' || id WHERE position('_' in id) = 0;

-- dispatch_pools (dpl_)
UPDATE dispatch_pools SET id = 'dpl_' || id WHERE position('_' in id) = 0;

-- dispatch_jobs (djb_)
UPDATE dispatch_jobs SET id = 'djb_' || id WHERE position('_' in id) = 0;

-- dispatch_jobs_read (djr_)
UPDATE dispatch_jobs_read SET id = 'djr_' || id WHERE position('_' in id) = 0;

-- schemas (sch_)
UPDATE schemas SET id = 'sch_' || id WHERE position('_' in id) = 0;

-- oauth_clients (oac_)
UPDATE oauth_clients SET id = 'oac_' || id WHERE position('_' in id) = 0;

-- authorization_codes (acd_)
UPDATE authorization_codes SET id = 'acd_' || id WHERE position('_' in id) = 0;

-- idp_role_mappings (irm_)
UPDATE idp_role_mappings SET id = 'irm_' || id WHERE position('_' in id) = 0;

-- client_auth_configs (cac_)
UPDATE client_auth_configs SET id = 'cac_' || id WHERE position('_' in id) = 0;

-- application_client_configs (apc_)
UPDATE application_client_configs SET id = 'apc_' || id WHERE position('_' in id) = 0;

-- client_access_grants (gnt_)
UPDATE client_access_grants SET id = 'gnt_' || id WHERE position('_' in id) = 0;

-- anchor_domains (anc_)
UPDATE anchor_domains SET id = 'anc_' || id WHERE position('_' in id) = 0;

-- cors_allowed_origins (cor_)
UPDATE cors_allowed_origins SET id = 'cor_' || id WHERE position('_' in id) = 0;

-- audit_logs (aud_)
UPDATE audit_logs SET id = 'aud_' || id WHERE position('_' in id) = 0;

-- =============================================================================
-- Part 4: Update foreign key references
-- =============================================================================

-- Update client_id references (clt_)
UPDATE principals SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE events SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE events_read SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE subscriptions SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE dispatch_pools SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE dispatch_jobs SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE dispatch_jobs_read SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE client_auth_configs SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE client_auth_configs SET primary_client_id = 'clt_' || primary_client_id WHERE primary_client_id IS NOT NULL AND position('_' in primary_client_id) = 0;
UPDATE application_client_configs SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE client_access_grants SET client_id = 'clt_' || client_id WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;
UPDATE authorization_codes SET context_client_id = 'clt_' || context_client_id WHERE context_client_id IS NOT NULL AND position('_' in context_client_id) = 0;
UPDATE refresh_tokens SET context_client_id = 'clt_' || context_client_id WHERE context_client_id IS NOT NULL AND position('_' in context_client_id) = 0;

-- Update principal_id references (prn_)
UPDATE client_access_grants SET principal_id = 'prn_' || principal_id WHERE principal_id IS NOT NULL AND position('_' in principal_id) = 0;
UPDATE authorization_codes SET principal_id = 'prn_' || principal_id WHERE principal_id IS NOT NULL AND position('_' in principal_id) = 0;
UPDATE refresh_tokens SET principal_id = 'prn_' || principal_id WHERE principal_id IS NOT NULL AND position('_' in principal_id) = 0;
UPDATE audit_logs SET principal_id = 'prn_' || principal_id WHERE principal_id IS NOT NULL AND position('_' in principal_id) = 0;
UPDATE oauth_clients SET service_account_principal_id = 'prn_' || service_account_principal_id WHERE service_account_principal_id IS NOT NULL AND position('_' in service_account_principal_id) = 0;
UPDATE cors_allowed_origins SET created_by = 'prn_' || created_by WHERE created_by IS NOT NULL AND position('_' in created_by) = 0;

-- Update application_id references (app_)
UPDATE principals SET application_id = 'app_' || application_id WHERE application_id IS NOT NULL AND position('_' in application_id) = 0;
UPDATE service_accounts SET application_id = 'app_' || application_id WHERE application_id IS NOT NULL AND position('_' in application_id) = 0;
UPDATE auth_roles SET application_id = 'app_' || application_id WHERE application_id IS NOT NULL AND position('_' in application_id) = 0;
UPDATE auth_permissions SET application_id = 'app_' || application_id WHERE application_id IS NOT NULL AND position('_' in application_id) = 0;
UPDATE application_client_configs SET application_id = 'app_' || application_id WHERE application_id IS NOT NULL AND position('_' in application_id) = 0;

-- Update service_account_id references (sac_)
UPDATE applications SET service_account_id = 'sac_' || service_account_id WHERE service_account_id IS NOT NULL AND position('_' in service_account_id) = 0;
UPDATE subscriptions SET service_account_id = 'sac_' || service_account_id WHERE service_account_id IS NOT NULL AND position('_' in service_account_id) = 0;
UPDATE dispatch_jobs SET service_account_id = 'sac_' || service_account_id WHERE service_account_id IS NOT NULL AND position('_' in service_account_id) = 0;
UPDATE dispatch_jobs_read SET service_account_id = 'sac_' || service_account_id WHERE service_account_id IS NOT NULL AND position('_' in service_account_id) = 0;

-- Update event_id references (events use raw TSIDs without prefix for performance)
-- events_read.event_id removed - id IS the event id
-- dispatch_jobs.event_id and dispatch_jobs_read.event_id reference unprefixed event IDs

-- Update event_type_id references (evt_)
UPDATE schemas SET event_type_id = 'evt_' || event_type_id WHERE event_type_id IS NOT NULL AND position('_' in event_type_id) = 0;

-- Update subscription_id references (sub_)
UPDATE dispatch_jobs SET subscription_id = 'sub_' || subscription_id WHERE subscription_id IS NOT NULL AND position('_' in subscription_id) = 0;
UPDATE dispatch_jobs_read SET subscription_id = 'sub_' || subscription_id WHERE subscription_id IS NOT NULL AND position('_' in subscription_id) = 0;

-- Update dispatch_pool_id references (dpl_)
UPDATE subscriptions SET dispatch_pool_id = 'dpl_' || dispatch_pool_id WHERE dispatch_pool_id IS NOT NULL AND position('_' in dispatch_pool_id) = 0;
UPDATE dispatch_jobs SET dispatch_pool_id = 'dpl_' || dispatch_pool_id WHERE dispatch_pool_id IS NOT NULL AND position('_' in dispatch_pool_id) = 0;
UPDATE dispatch_jobs_read SET dispatch_pool_id = 'dpl_' || dispatch_pool_id WHERE dispatch_pool_id IS NOT NULL AND position('_' in dispatch_pool_id) = 0;

-- Update schema_id references (sch_)
UPDATE dispatch_jobs SET schema_id = 'sch_' || schema_id WHERE schema_id IS NOT NULL AND position('_' in schema_id) = 0;

-- Update auth_config_id references (cac_)
UPDATE oidc_login_states SET auth_config_id = 'cac_' || auth_config_id WHERE auth_config_id IS NOT NULL AND position('_' in auth_config_id) = 0;

-- =============================================================================
-- Part 5: Update TEXT[] columns containing client IDs
-- =============================================================================

-- service_accounts.client_ids array
UPDATE service_accounts
SET client_ids = (
    SELECT array_agg(
        CASE WHEN position('_' in elem) = 0 THEN 'clt_' || elem ELSE elem END
    )
    FROM unnest(client_ids) AS elem
)
WHERE client_ids IS NOT NULL AND array_length(client_ids, 1) > 0;

-- client_auth_configs.additional_client_ids array
UPDATE client_auth_configs
SET additional_client_ids = (
    SELECT array_agg(
        CASE WHEN position('_' in elem) = 0 THEN 'clt_' || elem ELSE elem END
    )
    FROM unnest(additional_client_ids) AS elem
)
WHERE additional_client_ids IS NOT NULL AND array_length(additional_client_ids, 1) > 0;

-- client_auth_configs.granted_client_ids array
UPDATE client_auth_configs
SET granted_client_ids = (
    SELECT array_agg(
        CASE WHEN position('_' in elem) = 0 THEN 'clt_' || elem ELSE elem END
    )
    FROM unnest(granted_client_ids) AS elem
)
WHERE granted_client_ids IS NOT NULL AND array_length(granted_client_ids, 1) > 0;

-- oauth_clients.application_ids array (app_)
UPDATE oauth_clients
SET application_ids = (
    SELECT array_agg(
        CASE WHEN position('_' in elem) = 0 THEN 'app_' || elem ELSE elem END
    )
    FROM unnest(application_ids) AS elem
)
WHERE application_ids IS NOT NULL AND array_length(application_ids, 1) > 0;

-- =============================================================================
-- Part 6: Update audit_logs.entity_id based on entity_type
-- =============================================================================

-- Map entity types to their prefixes for existing data
UPDATE audit_logs SET entity_id = 'prn_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('Principal', 'User', 'ServiceAccount');

UPDATE audit_logs SET entity_id = 'clt_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'Client';

UPDATE audit_logs SET entity_id = 'app_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'Application';

UPDATE audit_logs SET entity_id = 'sac_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'Serviceaccount';

UPDATE audit_logs SET entity_id = 'anc_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('AnchorDomain', 'Anchordomain');

UPDATE audit_logs SET entity_id = 'rol_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('Role', 'AuthRole', 'Authrole');

UPDATE audit_logs SET entity_id = 'prm_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('Permission', 'AuthPermission', 'Authpermission');

UPDATE audit_logs SET entity_id = 'evt_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('EventType', 'Eventtype');

UPDATE audit_logs SET entity_id = 'sub_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'Subscription';

UPDATE audit_logs SET entity_id = 'dpl_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('DispatchPool', 'Dispatchpool');

UPDATE audit_logs SET entity_id = 'djb_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('DispatchJob', 'Dispatchjob');

UPDATE audit_logs SET entity_id = 'oac_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('OAuthClient', 'Oauthclient');

UPDATE audit_logs SET entity_id = 'cac_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('ClientAuthConfig', 'Clientauthconfig');

UPDATE audit_logs SET entity_id = 'apc_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('ApplicationClientConfig', 'Applicationclientconfig');

UPDATE audit_logs SET entity_id = 'gnt_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('ClientAccessGrant', 'Clientaccessgrant');

UPDATE audit_logs SET entity_id = 'sch_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'Schema';

UPDATE audit_logs SET entity_id = 'cor_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('CorsAllowedOrigin', 'Corsallowedorigin');

UPDATE audit_logs SET entity_id = 'irm_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('IdpRoleMapping', 'Idprolemapping');

-- For any remaining unknown entity types, leave as-is (they might be edge cases)
-- They will be logged but won't break anything
