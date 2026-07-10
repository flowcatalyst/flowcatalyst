-- Queries for msg_scheduled_jobs. Schema (migration 021, application_id
-- added in migration 038) matches the Go entity 1:1 — straightforward port.
-- Column order in every SELECT/INSERT list must match the table's physical
-- column order (application_id last — appended by migration 038's ALTER
-- TABLE) so sqlc maps rows onto the shared MsgScheduledJob model instead of
-- generating a bespoke per-query Row type.

-- name: ScheduledJobFindByID :one
SELECT id, client_id, code, name, description, status, crons, timezone,
       payload, concurrent, tracks_completion, timeout_seconds,
       delivery_max_attempts, target_url, last_fired_at,
       created_at, updated_at, created_by, updated_by, version, application_id
FROM msg_scheduled_jobs
WHERE id = $1;

-- name: ScheduledJobFindByCodeClient :one
SELECT id, client_id, code, name, description, status, crons, timezone,
       payload, concurrent, tracks_completion, timeout_seconds,
       delivery_max_attempts, target_url, last_fired_at,
       created_at, updated_at, created_by, updated_by, version, application_id
FROM msg_scheduled_jobs
WHERE code = $1 AND client_id = $2;

-- name: ScheduledJobFindByCodePlatform :one
SELECT id, client_id, code, name, description, status, crons, timezone,
       payload, concurrent, tracks_completion, timeout_seconds,
       delivery_max_attempts, target_url, last_fired_at,
       created_at, updated_at, created_by, updated_by, version, application_id
FROM msg_scheduled_jobs
WHERE code = $1 AND client_id IS NULL;

-- name: ScheduledJobFindAll :many
SELECT id, client_id, code, name, description, status, crons, timezone,
       payload, concurrent, tracks_completion, timeout_seconds,
       delivery_max_attempts, target_url, last_fired_at,
       created_at, updated_at, created_by, updated_by, version, application_id
FROM msg_scheduled_jobs
ORDER BY code;

-- name: ScheduledJobFindActive :many
SELECT id, client_id, code, name, description, status, crons, timezone,
       payload, concurrent, tracks_completion, timeout_seconds,
       delivery_max_attempts, target_url, last_fired_at,
       created_at, updated_at, created_by, updated_by, version, application_id
FROM msg_scheduled_jobs
WHERE status = 'ACTIVE';

-- name: ScheduledJobUpsert :exec
INSERT INTO msg_scheduled_jobs
    (id, client_id, code, name, description, status, crons, timezone,
     payload, concurrent, tracks_completion, timeout_seconds,
     delivery_max_attempts, target_url, last_fired_at,
     created_at, updated_at, created_by, updated_by, version, application_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
        $13, $14, $15, $16, $17, $18, $19, $20, $21)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    crons = EXCLUDED.crons,
    timezone = EXCLUDED.timezone,
    payload = EXCLUDED.payload,
    concurrent = EXCLUDED.concurrent,
    tracks_completion = EXCLUDED.tracks_completion,
    timeout_seconds = EXCLUDED.timeout_seconds,
    delivery_max_attempts = EXCLUDED.delivery_max_attempts,
    target_url = EXCLUDED.target_url,
    last_fired_at = EXCLUDED.last_fired_at,
    updated_at = EXCLUDED.updated_at,
    updated_by = EXCLUDED.updated_by,
    version = EXCLUDED.version;

-- name: ScheduledJobDelete :exec
DELETE FROM msg_scheduled_jobs WHERE id = $1;
