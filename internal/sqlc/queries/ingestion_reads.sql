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
