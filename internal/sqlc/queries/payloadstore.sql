-- Queries for msg_payloads, the payload table (docs/payload-offload-design.md,
-- sections 3 and 9). Owned by internal/platform/shared/payloadstore. A row is
-- inserted once and never updated, and always read by its primary key with
-- the created_at of the row that points at it.

-- name: PayloadFindByKey :one
SELECT id, created_at, data, ref, size
FROM msg_payloads
WHERE id = $1 AND created_at = $2;
