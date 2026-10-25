-- +goose NO TRANSACTION
-- +goose Up
-- Function trace logs, remedies after review (docs/function-trace-design.md
-- section 12, "Remedies 2026-10-25"). Re-runnable: every statement is
-- IF [NOT] EXISTS or idempotent. No transaction, because the indexes on
-- fng_trace_log are built CONCURRENTLY (docs/large-table-schema-changes.md);
-- a failed concurrent build leaves an INVALID index: DROP INDEX CONCURRENTLY
-- it and run the migration again.

-- 1. "Until switched off" is a flag with a hard ceiling, not a far-future
--    sentinel. A window stored with the old sentinel (enabled_until in year
--    9999) becomes an indefinite window ending 7 days from now (the default
--    FC_FUNCTIONS_TRACE_INDEFINITE_MAX_DAYS). The http context never takes
--    an indefinite window: one stored with the sentinel is cut to 15 minutes
--    from now (the default FC_FUNCTIONS_TRACE_HTTP_MAX_MINUTES).
ALTER TABLE fng_trace_contexts ADD COLUMN IF NOT EXISTS indefinite boolean NOT NULL DEFAULT false;

UPDATE fng_trace_contexts
   SET indefinite = true, enabled_until = now() + interval '7 days', updated_at = now()
 WHERE enabled_until >= timestamptz '9000-01-01 00:00:00+00' AND context <> 'http';

UPDATE fng_trace_contexts
   SET indefinite = false, enabled_until = now() + interval '15 minutes', updated_at = now()
 WHERE enabled_until >= timestamptz '9000-01-01 00:00:00+00' AND context = 'http';

-- 2. Retention prunes by the platform-written created_at, never the
--    runner-supplied `at`. (created_at) serves the sweep across functions,
--    (function_id, created_at) the per-function prune; the (at) index only
--    served the old retention scan and goes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_fng_trace_log_created_at
    ON fng_trace_log (created_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_fng_trace_log_function_created_at
    ON fng_trace_log (function_id, created_at);
DROP INDEX CONCURRENTLY IF EXISTS idx_fng_trace_log_at;

-- 3. platform:function:trace:indefinite (switch a declared context on "until
--    switched off") goes to the built-in role that manages tracing,
--    platform:messaging-admin, once (platform:super-admin has it through its
--    wildcard). Deliberately not to every role holding trace:manage: a custom
--    role is given it by hand.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, 'platform:function:trace:indefinite'
  FROM iam_roles r
 WHERE r.name = 'platform:messaging-admin'
ON CONFLICT DO NOTHING;

-- 4. platform:viewer no longer reads trace records (they hold what a
--    function printed and its business keys); the roles that operate
--    functions keep platform:function:trace:view.
DELETE FROM iam_role_permissions rp
 USING iam_roles r
 WHERE r.id = rp.role_id
   AND r.name = 'platform:viewer'
   AND rp.permission = 'platform:function:trace:view';

-- +goose Down
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, 'platform:function:trace:view'
  FROM iam_roles r
 WHERE r.name = 'platform:viewer'
ON CONFLICT DO NOTHING;

DELETE FROM iam_role_permissions WHERE permission = 'platform:function:trace:indefinite';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_fng_trace_log_at ON fng_trace_log (at);
DROP INDEX CONCURRENTLY IF EXISTS idx_fng_trace_log_function_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_fng_trace_log_created_at;

ALTER TABLE fng_trace_contexts DROP COLUMN IF EXISTS indefinite;
