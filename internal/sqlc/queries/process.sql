-- Queries for msg_processes. The schema has no created_by column —
-- matches the Rust source which hard-codes CreatedBy: None on read.

-- name: ProcessFindByID :one
SELECT id, code, name, description, status, source, application, subdomain,
       process_name, body, diagram_type, tags, created_at, updated_at
FROM msg_processes
WHERE id = $1;

-- name: ProcessFindByCode :one
SELECT id, code, name, description, status, source, application, subdomain,
       process_name, body, diagram_type, tags, created_at, updated_at
FROM msg_processes
WHERE code = $1;

-- name: ProcessUpsert :exec
INSERT INTO msg_processes
    (id, code, name, description, status, source, application, subdomain,
     process_name, body, diagram_type, tags, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    body = EXCLUDED.body,
    diagram_type = EXCLUDED.diagram_type,
    tags = EXCLUDED.tags,
    updated_at = EXCLUDED.updated_at;

-- name: ProcessDelete :exec
DELETE FROM msg_processes WHERE id = $1;
