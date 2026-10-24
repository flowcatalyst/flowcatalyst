-- Reads the operator API makes beside the ingestion aggregate: the failures
-- tab and the source kinds the live runtimes report.

-- The failures tab pages by id, newest first (idx_ing_failures_ingestion).
-- name: IngestionFailureList :many
SELECT * FROM ing_failures
WHERE ingestion_id = @ingestion_id
  AND (@after::text = '' OR id < @after::text)
ORDER BY id DESC
LIMIT @lim;

-- name: IngestionRuntimeKindsSeenSince :many
SELECT kinds FROM ing_runtimes WHERE last_seen_at >= $1;

-- The activity tab: what each runtime last said about the ingestion.
-- name: IngestionRouteStateList :many
SELECT * FROM ing_route_states WHERE ingestion_id = $1 ORDER BY runtime_id;

-- Exchange counts over the last day and the last week, from the windows the
-- runtimes reported. A window counts in the period its start falls in.
-- name: IngestionExchangeTotals :one
SELECT
    COALESCE(SUM(completed) FILTER (WHERE window_start >= @since_day::timestamptz), 0)::bigint AS completed_day,
    COALESCE(SUM(failed)    FILTER (WHERE window_start >= @since_day::timestamptz), 0)::bigint AS failed_day,
    COALESCE(SUM(records)   FILTER (WHERE window_start >= @since_day::timestamptz), 0)::bigint AS records_day,
    COALESCE(SUM(jobs)      FILTER (WHERE window_start >= @since_day::timestamptz), 0)::bigint AS jobs_day,
    COALESCE(SUM(completed), 0)::bigint AS completed_week,
    COALESCE(SUM(failed), 0)::bigint    AS failed_week,
    COALESCE(SUM(records), 0)::bigint   AS records_week,
    COALESCE(SUM(jobs), 0)::bigint      AS jobs_week
FROM ing_exchange_metrics
WHERE ingestion_id = @ingestion_id AND window_start >= @since_week::timestamptz;

-- name: IngestionLastExchange :one
SELECT window_start FROM ing_exchange_metrics
WHERE ingestion_id = $1 AND window_start >= $2 AND completed + failed > 0
ORDER BY window_start DESC LIMIT 1;

-- name: IngestionLastFailure :one
SELECT occurred_at FROM ing_failures WHERE ingestion_id = $1 ORDER BY id DESC LIMIT 1;
