-- +goose Up
-- Function trace logs (docs/function-trace-design.md section 6): opt-in,
-- time-boxed capture per (function, context). No foreign keys, the function
-- tables' convention (migration 059): a deleted function's trace rows are
-- left for retention to sweep.
--
-- fng_trace_contexts - one row per (function, context) ever switched on.
--                      The window is live iff enabled_until > now(), compared
--                      when read, so expiry needs no job.
-- fng_trace_log      - append-only records shipped by the runners.

CREATE TABLE IF NOT EXISTS fng_trace_contexts (
    function_id   TEXT        NOT NULL,
    context       TEXT        NOT NULL,
    enabled_until TIMESTAMPTZ,
    sample_rate   INTEGER     NOT NULL DEFAULT 1,
    enabled_by    TEXT,
    updated_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (function_id, context)
);

CREATE TABLE IF NOT EXISTS fng_trace_log (
    id             TEXT           PRIMARY KEY,
    function_id    TEXT           NOT NULL,
    version        INTEGER        NOT NULL,
    invocation_id  TEXT           NOT NULL,
    correlation_id TEXT,
    reference      TEXT,
    context        TEXT           NOT NULL,
    kind           TEXT           NOT NULL,
    level          TEXT           NOT NULL,
    message        TEXT           NOT NULL,
    data           JSONB,
    at             TIMESTAMPTZ(6) NOT NULL,
    runner_id      TEXT,
    created_at     TIMESTAMPTZ    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_fng_trace_log_function_at
    ON fng_trace_log (function_id, at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fng_trace_log_function_reference
    ON fng_trace_log (function_id, reference, at DESC) WHERE reference IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_fng_trace_log_function_correlation
    ON fng_trace_log (function_id, correlation_id, at DESC) WHERE correlation_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_fng_trace_log_function_invocation
    ON fng_trace_log (function_id, invocation_id);
CREATE INDEX IF NOT EXISTS idx_fng_trace_log_function_context
    ON fng_trace_log (function_id, context, at DESC);
CREATE INDEX IF NOT EXISTS idx_fng_trace_log_at
    ON fng_trace_log (at);

-- New permissions go to the roles that hold the matching function
-- permissions, once (the seeder never rewrites an existing role):
--   platform:function:trace:view          <- every role holding function:view
--   platform:function:trace:manage        <- every role holding function:manage
--   platform:function:trace:view-bodies   <- every role holding function:manage
-- View-bodies stays with the operators who may switch tracing on; viewers
-- and publishers read records with request/response bodies withheld.
-- Not granted to platform:function-deployer, the CI role pinned to "nothing
-- else" (a pipeline has no business reading request bodies).
-- Tokens carry permissions from mint time: sign in again to pick them up.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT rp.role_id, p.permission
  FROM iam_role_permissions rp
  JOIN iam_roles r ON r.id = rp.role_id
 CROSS JOIN (VALUES ('platform:function:trace:view')) AS p(permission)
 WHERE rp.permission = 'platform:function:function:view'
   AND r.name <> 'platform:function-deployer'
ON CONFLICT DO NOTHING;

INSERT INTO iam_role_permissions (role_id, permission)
SELECT rp.role_id, p.permission
  FROM iam_role_permissions rp
  JOIN iam_roles r ON r.id = rp.role_id
 CROSS JOIN (VALUES
        ('platform:function:trace:manage'),
        ('platform:function:trace:view-bodies')
      ) AS p(permission)
 WHERE rp.permission = 'platform:function:function:manage'
   AND r.name <> 'platform:function-deployer'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission IN (
        'platform:function:trace:view',
        'platform:function:trace:manage',
        'platform:function:trace:view-bodies');
DROP TABLE IF EXISTS fng_trace_log;
DROP TABLE IF EXISTS fng_trace_contexts;
