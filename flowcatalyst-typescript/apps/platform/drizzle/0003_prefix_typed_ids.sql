-- 0003: Add 3-character prefixes to all typed IDs
-- Format: {prefix}_{tsid} (e.g., "clt_0HZXEQ5Y8JY5Z")
-- Total length: 17 characters (3 prefix + 1 underscore + 13 TSID)
--
-- This follows the Stripe pattern where typed IDs are stored WITH the prefix
-- in the database, eliminating serialization/deserialization overhead.
--
-- NOTE: We use position('_' in col) = 0 instead of NOT LIKE '%_%' because
-- underscore is a wildcard in SQL LIKE patterns.
--
-- Entity Type Prefixes:
--   CLIENT = 'clt'
--   PRINCIPAL = 'prn'
--   APPLICATION = 'app'
--   SERVICE_ACCOUNT = 'sac'
--   ROLE = 'rol'
--   PERMISSION = 'prm'
--   OAUTH_CLIENT = 'oac'
--   AUTH_CODE = 'acd'
--   CLIENT_AUTH_CONFIG = 'cac'
--   APP_CLIENT_CONFIG = 'apc'
--   IDP_ROLE_MAPPING = 'irm'
--   CORS_ORIGIN = 'cor'
--   ANCHOR_DOMAIN = 'anc'
--   CLIENT_ACCESS_GRANT = 'gnt'
--   EVENT_TYPE = 'evt'
--   EVENT = 'evn'
--   EVENT_READ = 'evr'
--   SUBSCRIPTION = 'sub'
--   DISPATCH_POOL = 'dpl'
--   DISPATCH_JOB = 'djb'
--   DISPATCH_JOB_READ = 'djr'
--   SCHEMA = 'sch'
--   AUDIT_LOG = 'aud'

-- =============================================================================
-- Part 1: Expand VARCHAR columns from 13 to 17 characters
-- =============================================================================

-- Principals table
ALTER TABLE principals ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE principals ALTER COLUMN client_id TYPE VARCHAR(17);
ALTER TABLE principals ALTER COLUMN application_id TYPE VARCHAR(17);

-- Clients table
ALTER TABLE clients ALTER COLUMN id TYPE VARCHAR(17);

-- Anchor domains table
ALTER TABLE anchor_domains ALTER COLUMN id TYPE VARCHAR(17);

-- Events table
ALTER TABLE events ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE events ALTER COLUMN causation_id TYPE VARCHAR(17);
ALTER TABLE events ALTER COLUMN client_id TYPE VARCHAR(17);

-- Audit logs table
ALTER TABLE audit_logs ALTER COLUMN id TYPE VARCHAR(17);
ALTER TABLE audit_logs ALTER COLUMN entity_id TYPE VARCHAR(17);
ALTER TABLE audit_logs ALTER COLUMN principal_id TYPE VARCHAR(17);

-- =============================================================================
-- Part 2: Add prefixes to existing IDs (primary keys first)
-- =============================================================================

-- Clients (clt_) - must be updated before principals.client_id FK
UPDATE clients SET id = 'clt_' || id WHERE position('_' in id) = 0;

-- Principals (prn_)
UPDATE principals SET id = 'prn_' || id WHERE position('_' in id) = 0;

-- Anchor domains (anc_)
UPDATE anchor_domains SET id = 'anc_' || id WHERE position('_' in id) = 0;

-- Events (evn_)
UPDATE events SET id = 'evn_' || id WHERE position('_' in id) = 0;

-- Audit logs (aud_)
UPDATE audit_logs SET id = 'aud_' || id WHERE position('_' in id) = 0;

-- =============================================================================
-- Part 3: Update foreign key references
-- =============================================================================

-- principals.client_id -> clt_
UPDATE principals SET client_id = 'clt_' || client_id
WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;

-- principals.application_id -> app_
UPDATE principals SET application_id = 'app_' || application_id
WHERE application_id IS NOT NULL AND position('_' in application_id) = 0;

-- events.client_id -> clt_
UPDATE events SET client_id = 'clt_' || client_id
WHERE client_id IS NOT NULL AND position('_' in client_id) = 0;

-- events.causation_id -> evn_ (only if it looks like a raw TSID, not trace-xxx)
UPDATE events SET causation_id = 'evn_' || causation_id
WHERE causation_id IS NOT NULL
  AND position('_' in causation_id) = 0
  AND causation_id NOT LIKE 'trace-%'
  AND causation_id NOT LIKE 'exec-%';

-- audit_logs.principal_id -> prn_
UPDATE audit_logs SET principal_id = 'prn_' || principal_id
WHERE principal_id IS NOT NULL AND position('_' in principal_id) = 0;

-- =============================================================================
-- Part 4: Update audit_logs.entity_id based on entity_type
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

UPDATE audit_logs SET entity_id = 'anc_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'AnchorDomain';

UPDATE audit_logs SET entity_id = 'rol_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type IN ('Role', 'AuthRole');

UPDATE audit_logs SET entity_id = 'evt_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'EventType';

UPDATE audit_logs SET entity_id = 'sub_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'Subscription';

UPDATE audit_logs SET entity_id = 'dpl_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'DispatchPool';

UPDATE audit_logs SET entity_id = 'oac_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'OAuthClient';

UPDATE audit_logs SET entity_id = 'cac_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'ClientAuthConfig';

UPDATE audit_logs SET entity_id = 'gnt_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'ClientAccessGrant';

UPDATE audit_logs SET entity_id = 'sch_' || entity_id
WHERE position('_' in entity_id) = 0
  AND entity_type = 'Schema';

-- For any remaining unknown entity types, leave as-is (they might be edge cases)
-- They will be logged but won't break anything
