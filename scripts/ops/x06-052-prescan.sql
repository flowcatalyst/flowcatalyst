-- X-06 / migration 052 PRODUCTION PRE-SCAN
--
-- Run this against PRODUCTION (read-only) BEFORE deploying migration
-- 052_x06_dispatch_job_enum_check_constraints.sql. The migration's own pre-scan ran
-- against a freshly-migrated TEST database, which proves nothing about real data: any
-- row holding a value outside the allowed set below (which already includes the known
-- legacy aliases IN_PROGRESS/ERROR and IMMEDIATE/FIXED_DELAY) will make the migration
-- FAIL TO APPLY.
--
-- This is not hypothetical — see scripts/ops/x06-051-prescan.sql's header for the
-- precedent (a test fixture seeding a value that never existed in the Go type,
-- surviving only because the pre-X-06 parsers silently coerced it on read).
--
-- EXPECTED RESULT: zero rows. Any row returned names a table, column, offending
-- value and a count. For each one, decide before deploying:
--   * legitimate legacy value  -> widen that CHECK in 052 to admit it
--   * genuine corruption       -> correct the data, then deploy
-- Do NOT widen a constraint to make a scan pass without deciding which of those it is.

SELECT 'msg_dispatch_jobs' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_dispatch_jobs
 WHERE status IS NOT NULL
   AND status NOT IN ('PENDING', 'QUEUED', 'PROCESSING', 'IN_PROGRESS', 'COMPLETED', 'FAILED', 'ERROR', 'CANCELLED', 'EXPIRED')
 GROUP BY status
UNION ALL
SELECT 'msg_dispatch_jobs' AS table_name, 'kind' AS column_name, kind::text AS bad_value, count(*) AS rows
  FROM msg_dispatch_jobs
 WHERE kind IS NOT NULL AND kind NOT IN ('EVENT', 'TASK')
 GROUP BY kind
UNION ALL
SELECT 'msg_dispatch_jobs' AS table_name, 'retry_strategy' AS column_name, retry_strategy::text AS bad_value, count(*) AS rows
  FROM msg_dispatch_jobs
 WHERE retry_strategy IS NOT NULL
   AND retry_strategy NOT IN ('immediate', 'IMMEDIATE', 'fixed', 'FIXED_DELAY', 'exponential')
 GROUP BY retry_strategy
UNION ALL
SELECT 'msg_dispatch_jobs_read' AS table_name, 'status' AS column_name, status::text AS bad_value, count(*) AS rows
  FROM msg_dispatch_jobs_read
 WHERE status IS NOT NULL
   AND status NOT IN ('PENDING', 'QUEUED', 'PROCESSING', 'IN_PROGRESS', 'COMPLETED', 'FAILED', 'ERROR', 'CANCELLED', 'EXPIRED')
 GROUP BY status
UNION ALL
SELECT 'msg_dispatch_jobs_read' AS table_name, 'kind' AS column_name, kind::text AS bad_value, count(*) AS rows
  FROM msg_dispatch_jobs_read
 WHERE kind IS NOT NULL AND kind NOT IN ('EVENT', 'TASK')
 GROUP BY kind
UNION ALL
SELECT 'msg_dispatch_jobs_read' AS table_name, 'retry_strategy' AS column_name, retry_strategy::text AS bad_value, count(*) AS rows
  FROM msg_dispatch_jobs_read
 WHERE retry_strategy IS NOT NULL
   AND retry_strategy NOT IN ('immediate', 'IMMEDIATE', 'fixed', 'FIXED_DELAY', 'exponential')
 GROUP BY retry_strategy
UNION ALL
SELECT 'msg_dispatch_job_attempts' AS table_name, 'error_type' AS column_name, error_type::text AS bad_value, count(*) AS rows
  FROM msg_dispatch_job_attempts
 WHERE error_type IS NOT NULL
   AND error_type NOT IN ('CONNECTION', 'TIMEOUT', 'HTTP_ERROR', 'VALIDATION', 'UNKNOWN')
 GROUP BY error_type
ORDER BY table_name, column_name;
