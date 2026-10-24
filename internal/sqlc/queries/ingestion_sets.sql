-- Queries for sets of ingestions (docs/ingestion-design.md section 3): ing_sets
-- and ing_set_applies, and the reads of the ingestions a set owns.

-- name: IngestionSetFindByName :one
SELECT * FROM ing_sets WHERE name = $1;

-- name: IngestionSetFindByIDs :many
SELECT * FROM ing_sets WHERE id = ANY(@ids::text[]);

-- name: IngestionSetInsert :exec
INSERT INTO ing_sets (id, name, created_at, updated_at) VALUES ($1, $2, $3, $4);

-- name: IngestionSetTouch :exec
UPDATE ing_sets SET updated_at = $2 WHERE id = $1;

-- Every set with how many ingestions it owns.
-- name: IngestionSetList :many
SELECT s.id, s.name, s.created_at, s.updated_at, count(i.id) AS ingestions
FROM ing_sets s
LEFT JOIN ing_ingestions i ON i.managed_by = s.id
GROUP BY s.id
ORDER BY s.name;

-- name: IngestionListManagedBy :many
SELECT * FROM ing_ingestions WHERE managed_by = $1 ORDER BY client_id, code;

-- name: IngestionSetApplyInsert :exec
INSERT INTO ing_set_applies (id, set_id, archive_uri, summary, applied_by, applied_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: IngestionSetApplyList :many
SELECT * FROM ing_set_applies WHERE set_id = $1 ORDER BY applied_at DESC, id DESC;

-- name: IngestionSetApplyFind :one
SELECT * FROM ing_set_applies WHERE set_id = $1 AND id = $2;

-- Forgets the applies beyond the newest @keep of a set, returning where their
-- archives were kept.
-- name: IngestionSetApplyForget :many
DELETE FROM ing_set_applies
WHERE ing_set_applies.set_id = sqlc.arg(set_id)::text
  AND ing_set_applies.id NOT IN (
      SELECT a.id FROM ing_set_applies a WHERE a.set_id = sqlc.arg(set_id)::text
      ORDER BY a.applied_at DESC, a.id DESC LIMIT sqlc.arg(keep)::int)
RETURNING archive_uri;
