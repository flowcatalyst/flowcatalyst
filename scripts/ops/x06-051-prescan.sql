-- X-06 / migration 051 PRODUCTION PRE-SCAN
--
-- Run this against PRODUCTION (read-only) BEFORE deploying migration
-- 051_x06_enum_check_constraints.sql. The migration's own pre-scan ran against a
-- freshly-migrated TEST database, which proves nothing about real data: any row
-- holding a value outside the Go const block will make the migration FAIL TO APPLY.
--
-- This is not hypothetical. A test fixture in this repo was found seeding
-- iam_principals.scope = 'PLATFORM' — a value that never existed in the Go type and
-- survived only because the pre-X-06 parsers silently coerced unknown values on read.
-- Lenient parsing is exactly how invalid values accumulate unnoticed.
--
-- EXPECTED RESULT: zero rows. Any row returned names a table, column, offending
-- value and a count. For each one, decide before deploying:
--   * legitimate legacy value  -> widen that CHECK in 051 to admit it
--                                 (precedent: msg_dispatch_jobs.status must admit the
--                                  legacy 'ERROR', which GroupHoldingStatusSQL still
--                                  matches by design — that column is NOT in 051, but
--                                  it is the pattern to look for here)
--   * genuine corruption       -> correct the data, then deploy
-- Do NOT widen a constraint to make a scan pass without deciding which of those it is.

SELECT 'iam_service_accounts' AS table_name, 'wh_auth_type' AS column_name, wh_auth_type::text AS bad_value, count(*) AS rows
  FROM iam_service_accounts WHERE wh_auth_type IS NOT NULL AND wh_auth_type NOT IN ('NONE', 'BEARER_TOKEN', 'BASIC_AUTH', 'API_KEY', 'HMAC_SIGNATURE') GROUP BY wh_auth_type
UNION ALL
SELECT 'iam_login_attempts' AS table_name, 'outcome' AS column_name, outcome::text AS bad_value, count(*) AS rows
  FROM iam_login_attempts WHERE outcome IS NOT NULL AND outcome NOT IN ('SUCCESS', 'FAILURE') GROUP BY outcome
UNION ALL
SELECT 'iam_password_reset_tokens' AS table_name, 'purpose' AS column_name, purpose::text AS bad_value, count(*) AS rows
  FROM iam_password_reset_tokens WHERE purpose IS NOT NULL AND purpose NOT IN ('reset', 'invite') GROUP BY purpose
UNION ALL
SELECT 'msg_dispatch_pools' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_dispatch_pools WHERE status IS NOT NULL AND status NOT IN ('ACTIVE', 'SUSPENDED', 'ARCHIVED') GROUP BY status
UNION ALL
SELECT 'tnt_clients' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM tnt_clients WHERE status IS NOT NULL AND status NOT IN ('ACTIVE', 'INACTIVE', 'SUSPENDED') GROUP BY status
UNION ALL
SELECT 'msg_processes' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_processes WHERE status IS NOT NULL AND status NOT IN ('CURRENT', 'ARCHIVED') GROUP BY status
UNION ALL
SELECT 'msg_processes' AS table_name, 'source' AS column_name, source::text AS bad_value, count(*) AS rows
  FROM msg_processes WHERE source IS NOT NULL AND source NOT IN ('CODE', 'API', 'UI') GROUP BY source
UNION ALL
SELECT 'app_applications' AS table_name, 'type' AS column_name, type::text AS bad_value, count(*) AS rows
  FROM app_applications WHERE type IS NOT NULL AND type NOT IN ('APPLICATION', 'INTEGRATION') GROUP BY type
UNION ALL
SELECT 'msg_subscriptions' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_subscriptions WHERE status IS NOT NULL AND status NOT IN ('ACTIVE', 'PAUSED') GROUP BY status
UNION ALL
SELECT 'msg_subscriptions' AS table_name, 'source' AS column_name, source::text AS bad_value, count(*) AS rows
  FROM msg_subscriptions WHERE source IS NOT NULL AND source NOT IN ('CODE', 'API', 'UI') GROUP BY source
UNION ALL
SELECT 'msg_event_types' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_event_types WHERE status IS NOT NULL AND status NOT IN ('CURRENT', 'ARCHIVED') GROUP BY status
UNION ALL
SELECT 'msg_event_types' AS table_name, 'source' AS column_name, source::text AS bad_value, count(*) AS rows
  FROM msg_event_types WHERE source IS NOT NULL AND source NOT IN ('CODE', 'API', 'UI') GROUP BY source
UNION ALL
SELECT 'msg_event_type_spec_versions' AS table_name, 'schema_type' AS column_name, schema_type::text AS bad_value, count(*) AS rows
  FROM msg_event_type_spec_versions WHERE schema_type IS NOT NULL AND schema_type NOT IN ('JSON_SCHEMA', 'XSD', 'XML_SCHEMA', 'PROTO', 'PROTOBUF') GROUP BY schema_type
UNION ALL
SELECT 'msg_event_type_spec_versions' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_event_type_spec_versions WHERE status IS NOT NULL AND status NOT IN ('FINALISING', 'CURRENT', 'DEPRECATED') GROUP BY status
UNION ALL
SELECT 'iam_principals' AS table_name, 'type' AS column_name, type::text AS bad_value, count(*) AS rows
  FROM iam_principals WHERE type IS NOT NULL AND type NOT IN ('USER', 'SERVICE') GROUP BY type
UNION ALL
SELECT 'iam_principals' AS table_name, 'scope' AS column_name, scope::text AS bad_value, count(*) AS rows
  FROM iam_principals WHERE scope IS NOT NULL AND scope NOT IN ('ANCHOR', 'PARTNER', 'CLIENT') GROUP BY scope
UNION ALL
SELECT 'iam_roles' AS table_name, 'source' AS column_name, source::text AS bad_value, count(*) AS rows
  FROM iam_roles WHERE source IS NOT NULL AND source NOT IN ('CODE', 'DATABASE', 'SDK') GROUP BY source
UNION ALL
SELECT 'msg_connections' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_connections WHERE status IS NOT NULL AND status NOT IN ('ACTIVE', 'PAUSED') GROUP BY status
UNION ALL
SELECT 'oauth_clients' AS table_name, 'client_type' AS column_name, client_type::text AS bad_value, count(*) AS rows
  FROM oauth_clients WHERE client_type IS NOT NULL AND client_type NOT IN ('PUBLIC', 'CONFIDENTIAL') GROUP BY client_type
UNION ALL
SELECT 'tnt_client_auth_configs' AS table_name, 'config_type' AS column_name, config_type::text AS bad_value, count(*) AS rows
  FROM tnt_client_auth_configs WHERE config_type IS NOT NULL AND config_type NOT IN ('ANCHOR', 'PARTNER', 'CLIENT') GROUP BY config_type
UNION ALL
SELECT 'tnt_client_auth_configs' AS table_name, 'auth_provider' AS column_name, auth_provider::text AS bad_value, count(*) AS rows
  FROM tnt_client_auth_configs WHERE auth_provider IS NOT NULL AND auth_provider NOT IN ('INTERNAL', 'OIDC') GROUP BY auth_provider
UNION ALL
SELECT 'oauth_identity_providers' AS table_name, 'type' AS column_name, type::text AS bad_value, count(*) AS rows
  FROM oauth_identity_providers WHERE type IS NOT NULL AND type NOT IN ('INTERNAL', 'OIDC') GROUP BY type
UNION ALL
SELECT 'tnt_email_domain_mappings' AS table_name, 'scope_type' AS column_name, scope_type::text AS bad_value, count(*) AS rows
  FROM tnt_email_domain_mappings WHERE scope_type IS NOT NULL AND scope_type NOT IN ('ANCHOR', 'PARTNER', 'CLIENT') GROUP BY scope_type
UNION ALL
SELECT 'app_platform_configs' AS table_name, 'scope' AS column_name, scope::text AS bad_value, count(*) AS rows
  FROM app_platform_configs WHERE scope IS NOT NULL AND scope NOT IN ('GLOBAL', 'CLIENT') GROUP BY scope
UNION ALL
SELECT 'app_platform_configs' AS table_name, 'value_type' AS column_name, value_type::text AS bad_value, count(*) AS rows
  FROM app_platform_configs WHERE value_type IS NOT NULL AND value_type NOT IN ('PLAIN', 'SECRET') GROUP BY value_type
UNION ALL
SELECT 'app_application_openapi_specs' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM app_application_openapi_specs WHERE status IS NOT NULL AND status NOT IN ('CURRENT', 'ARCHIVED') GROUP BY status
UNION ALL
SELECT 'msg_scheduled_jobs' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_scheduled_jobs WHERE status IS NOT NULL AND status NOT IN ('ACTIVE', 'PAUSED', 'ARCHIVED') GROUP BY status
UNION ALL
SELECT 'msg_scheduled_job_instances' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_scheduled_job_instances WHERE status IS NOT NULL AND status NOT IN ('QUEUED', 'IN_FLIGHT', 'DELIVERED', 'COMPLETED', 'FAILED', 'DELIVERY_FAILED') GROUP BY status
UNION ALL
SELECT 'msg_scheduled_job_instances' AS table_name, 'trigger_kind' AS column_name, trigger_kind::text AS bad_value, count(*) AS rows
  FROM msg_scheduled_job_instances WHERE trigger_kind IS NOT NULL AND trigger_kind NOT IN ('CRON', 'MANUAL', 'BACKFILL') GROUP BY trigger_kind
ORDER BY table_name, column_name;
