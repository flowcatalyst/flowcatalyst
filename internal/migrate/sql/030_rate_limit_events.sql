-- FlowCatalyst — distributed rate-limit events
--
-- Postgres-backed implementation of the `RateLimitStore` trait. Used when
-- FC_REDIS_URL is unset or unreachable; the Redis backend handles the same
-- counting via INCR + EXPIRE on namespaced keys, so this table is just the
-- fallback path.
--
-- Schema mirrors `iam_login_attempts` (migration 008) — a single append-only
-- events table with a (bucket, key, occurred_at) shape. The middleware counts
-- rows in a sliding window via `COUNT(*) WHERE bucket = $1 AND key = $2 AND
-- occurred_at > now() - $3`. Append-only avoids row locks under contention.
--
-- `bucket` identifies the limiter (e.g. `oauth_token_ip`, `oauth_token_client`,
-- `password_reset_email`) so multiple policies coexist in one table without
-- key collisions. `key` is the limiter input (IP, client_id, email hash, …).
-- Both kept TEXT so callers can pass whatever shape suits the bucket.
--
-- Rows are reaped by the background prune job (`prune_rate_limit_events`)
-- — see PostgresRateLimitStore::prune. Without the prune, the table grows
-- unboundedly under sustained traffic; with it, max size ≈ peak QPS × window.

CREATE TABLE IF NOT EXISTS iam_rate_limit_events (
    id           BIGSERIAL PRIMARY KEY,
    bucket       VARCHAR(64) NOT NULL,
    key          TEXT        NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial-ish index for the hot path: COUNT(*) WHERE bucket=$1 AND key=$2
-- AND occurred_at > $3. Composite (bucket, key, occurred_at) so the planner
-- can index-only-scan the time bound after seeking on (bucket, key).
CREATE INDEX IF NOT EXISTS idx_iam_rate_limit_events_lookup
    ON iam_rate_limit_events (bucket, key, occurred_at DESC);

-- Reaper-friendly index: lets `DELETE … WHERE occurred_at < $1` walk by time.
CREATE INDEX IF NOT EXISTS idx_iam_rate_limit_events_occurred_at
    ON iam_rate_limit_events (occurred_at);
