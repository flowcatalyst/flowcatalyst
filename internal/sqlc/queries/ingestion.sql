-- Queries for ing_ingestions and ing_versions. The filtered, paged list is a
-- hand-rolled dynamic query in the repository (docs/sqlc.md).

-- name: IngestionFindByID :one
SELECT * FROM ing_ingestions WHERE id = $1;

-- name: IngestionFindByCode :one
SELECT * FROM ing_ingestions
WHERE application_id = $1 AND client_id = $2 AND code = $3;

-- name: IngestionInsert :exec
INSERT INTO ing_ingestions
    (id, application_id, client_id, code, name, active, version, latest_version,
     source, split, target, keep_failed_payloads, managed_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);

-- Guarded on the version pointer and the latest number the caller read: a
-- concurrent save or promote moves one of them and this updates no row.
-- name: IngestionUpdate :execrows
UPDATE ing_ingestions SET
    name = @name,
    active = @active,
    version = @version,
    latest_version = @latest_version,
    source = @source,
    split = @split,
    target = @target,
    keep_failed_payloads = @keep_failed_payloads,
    managed_by = @managed_by,
    updated_at = @updated_at
WHERE id = @id
  AND version = @loaded_version
  AND latest_version = @loaded_latest;

-- name: IngestionDelete :exec
DELETE FROM ing_ingestions WHERE id = $1;

-- The trace tables carry no foreign key (migration 094), so deleting an
-- ingestion removes its window and records itself, in the same transaction.
-- name: IngestionTraceWindowDelete :exec
DELETE FROM ing_trace_windows WHERE ingestion_id = $1;

-- name: IngestionTraceLogDelete :exec
DELETE FROM ing_trace_log WHERE ingestion_id = $1;

-- definition_digest is computed by the database from the definition's jsonb
-- text (Postgres's canonical form: keys sorted, no insignificant whitespace),
-- the same expression migration 093 backfilled with and
-- IngestionVersionFindByDigest looks up by: one canonicalisation.
-- name: IngestionVersionInsert :exec
INSERT INTO ing_versions
    (id, ingestion_id, version, definition, definition_digest, note, applied_from, created_by, created_at)
VALUES (@id, @ingestion_id, @version, @definition,
    encode(sha256(convert_to((@definition::jsonb)::text, 'UTF8')), 'hex'),
    @note, @applied_from, @created_by, @created_at);

-- The newest saved version of the ingestion whose definition says what this
-- one says (same digest, and the same jsonb, so a collision cannot match).
-- name: IngestionVersionFindByDigest :one
SELECT * FROM ing_versions
WHERE ingestion_id = @ingestion_id
  AND definition_digest = encode(sha256(convert_to((@definition::jsonb)::text, 'UTF8')), 'hex')
  AND definition = @definition::jsonb
ORDER BY version DESC
LIMIT 1;

-- name: IngestionVersionFind :one
SELECT * FROM ing_versions WHERE ingestion_id = $1 AND version = $2;

-- name: IngestionVersionList :many
SELECT * FROM ing_versions WHERE ingestion_id = $1 ORDER BY version DESC;
