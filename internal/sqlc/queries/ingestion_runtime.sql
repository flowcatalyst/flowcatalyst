-- Queries for the runtime API (docs/ingestion-design.md section 7): runtimes,
-- leases, state, reports and test requests. The live-ingestion read the poll
-- makes is hand-rolled in the repository (it shares the list's row type), and
-- so are the retention deletes (chunked, as for the function trace log).

-- name: IngestionRuntimeUpsert :exec
INSERT INTO ing_runtimes (runtime_id, kinds, contract_version, url, first_seen_at, last_seen_at)
VALUES (@runtime_id, @kinds, @contract_version, @url, @now, @now)
ON CONFLICT (runtime_id) DO UPDATE SET
    kinds = EXCLUDED.kinds,
    contract_version = EXCLUDED.contract_version,
    url = EXCLUDED.url,
    last_seen_at = EXCLUDED.last_seen_at;

-- The lease is taken in one statement: a conflicting row is taken over only by
-- its holder (a renewal) or once it has expired, and two runtimes racing for
-- the same ingestion are serialised on the row, so exactly one affects it.
-- name: IngestionLeaseClaim :execrows
INSERT INTO ing_leases (ingestion_id, runtime_id, acquired_at, expires_at, released)
VALUES (@ingestion_id, @runtime_id, @now, @expires_at, FALSE)
ON CONFLICT (ingestion_id) DO UPDATE SET
    acquired_at = CASE WHEN ing_leases.runtime_id = EXCLUDED.runtime_id AND ing_leases.expires_at > EXCLUDED.acquired_at
                       THEN ing_leases.acquired_at ELSE EXCLUDED.acquired_at END,
    runtime_id = EXCLUDED.runtime_id,
    expires_at = EXCLUDED.expires_at,
    released = FALSE
WHERE ing_leases.runtime_id = EXCLUDED.runtime_id OR ing_leases.expires_at <= EXCLUDED.acquired_at;

-- A runtime that stops running an ingestion marks its lease released; nobody
-- else takes it before it expires, because the holder may be finishing a batch.
-- name: IngestionLeaseReleaseExcept :execrows
UPDATE ing_leases SET released = TRUE
WHERE runtime_id = @runtime_id AND NOT released AND NOT (ingestion_id = ANY(@keep::text[]));

-- name: IngestionLeaseHeld :one
SELECT EXISTS (
    SELECT 1 FROM ing_leases
    WHERE ingestion_id = @ingestion_id AND runtime_id = @runtime_id AND expires_at > @now
);

-- The row is locked while the value is written, so the lease cannot run out and
-- pass to another runtime in between.
-- name: IngestionLeaseLockHeld :one
SELECT ingestion_id FROM ing_leases
WHERE ingestion_id = @ingestion_id AND runtime_id = @runtime_id AND expires_at > @now
FOR UPDATE;

-- name: IngestionStateGet :one
SELECT value FROM ing_state WHERE ingestion_id = $1 AND key = $2;

-- name: IngestionStatePut :exec
INSERT INTO ing_state (ingestion_id, key, value, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (ingestion_id, key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at;

-- name: IngestionFindByIDs :many
SELECT * FROM ing_ingestions WHERE id = ANY(@ids::text[]);

-- name: IngestionRouteStateUpsert :exec
INSERT INTO ing_route_states (ingestion_id, runtime_id, version, status, error, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (ingestion_id, runtime_id) DO UPDATE SET
    version = EXCLUDED.version, status = EXCLUDED.status, error = EXCLUDED.error, updated_at = EXCLUDED.updated_at;

-- A window re-sent replaces the counts: it never adds to them.
-- name: IngestionMetricUpsert :exec
INSERT INTO ing_exchange_metrics (ingestion_id, runtime_id, window_start, window_secs, completed, failed, records, jobs)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (ingestion_id, runtime_id, window_start) DO UPDATE SET
    window_secs = EXCLUDED.window_secs, completed = EXCLUDED.completed, failed = EXCLUDED.failed,
    records = EXCLUDED.records, jobs = EXCLUDED.jobs;

-- name: IngestionFailureInsert :exec
INSERT INTO ing_failures (id, ingestion_id, runtime_id, version, kind, message, after_records, excerpt, occurred_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: IngestionTestInsert :exec
INSERT INTO ing_test_requests
    (id, ingestion_id, application_id, client_id, kind, definition, status, requested_by, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', $7, $8, $9);

-- name: IngestionTestFind :one
SELECT * FROM ing_test_requests WHERE id = $1;

-- A runtime claims the pending requests for the kinds it carries; SKIP LOCKED
-- lets two runtimes polling together take different ones, never the same.
-- name: IngestionTestClaim :many
UPDATE ing_test_requests SET status = 'RUNNING', claimed_by = @runtime_id
WHERE id IN (
    SELECT t.id FROM ing_test_requests t
    WHERE t.status = 'PENDING' AND t.expires_at > @now AND t.kind = ANY(@kinds::text[])
      AND (sqlc.narg('clients')::text[] IS NULL OR t.client_id = ANY(sqlc.narg('clients')::text[]))
      AND (sqlc.narg('applications')::text[] IS NULL OR t.application_id = ANY(sqlc.narg('applications')::text[]))
    ORDER BY t.created_at
    LIMIT @lim
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- Only the runtime that claimed a request, while it is open, answers it.
-- name: IngestionTestComplete :execrows
UPDATE ing_test_requests SET status = 'DONE', result = @result, completed_at = @now
WHERE id = @id AND status = 'RUNNING' AND claimed_by = @runtime_id AND expires_at > @now;

-- What a claimed request is named by: its application, client and, for a saved
-- ingestion, its code.
-- name: IngestionTestRefs :many
SELECT t.id, a.code AS application_code, c.identifier AS client_identifier, i.code AS ingestion_code
FROM ing_test_requests t
JOIN app_applications a ON a.id = t.application_id
JOIN tnt_clients c ON c.id = t.client_id
LEFT JOIN ing_ingestions i ON i.id = t.ingestion_id
WHERE t.id = ANY(@ids::text[]);

-- name: IngestionRuntimeKinds :one
SELECT kinds FROM ing_runtimes WHERE runtime_id = $1;

-- The first request to name a key wins it; the caller reads back who did.
-- name: IngestionJobKeyClaim :execrows
INSERT INTO ing_job_keys (ingestion_id, external_id, job_id, created_at)
VALUES (@ingestion_id, @external_id, @job_id, @created_at)
ON CONFLICT (ingestion_id, external_id) DO NOTHING;

-- name: IngestionJobKeysFind :many
SELECT external_id, job_id FROM ing_job_keys
WHERE ingestion_id = @ingestion_id AND external_id = ANY(@external_ids::text[]);
