-- Read queries for msg_dispatch_jobs + the attempts table. The column set
-- matches the post-019 (partitioned) schema. Composite PK is (id, created_at).
--
-- msg_dispatch_jobs is WRITTEN only by the dispatch-job lifecycle
-- (internal/platform/dispatchjob/lifecycle.go): every insert and every status
-- transition is hand-built there, over whatever pool or transaction the caller
-- passes, so it carries no sqlc query. Nothing in this file may INSERT, UPDATE
-- or DELETE msg_dispatch_jobs (lifecycle_enforce_test.go checks).
--
-- FindWithFilters + DistinctValues stay hand-rolled in repository.go
-- (dynamic WHERE + dynamic column names).

-- name: DispatchJobFindByID :one
-- The job and, when it has a payload_id (the new shape of
-- docs/payload-offload-design.md, section 10), its msg_payloads row, joined by
-- primary key with the job's own created_at: payload_data is the payload held
-- in the database, payload_ref a reference into the payload store (read only
-- by a caller that needs the bytes, Repository.ResolvePayload), payload_size
-- its length. A legacy job (payload_id NULL) has its payload in `payload`.
SELECT j.id, j.external_id, j.source, j.kind, j.code, j.subject, j.event_id,
       j.correlation_id, j.metadata, j.target_url, j.protocol, j.payload,
       j.payload_content_type, j.data_only, j.service_account_id, j.client_id,
       j.subscription_id, j.mode, j.dispatch_pool_id, j.message_group, j.sequence,
       j.timeout_seconds, j.schema_id, j.status, j.max_retries, j.retry_strategy,
       j.scheduled_for, j.expires_at, j.attempt_count, j.last_attempt_at,
       j.completed_at, j.duration_millis, j.last_error, j.idempotency_key,
       j.queue, j.descriptor, j.connection_id, j.not_before, j.created_at, j.updated_at,
       j.payload_id, p.data AS payload_data, p.ref AS payload_ref, p.size AS payload_size
FROM msg_dispatch_jobs j
LEFT JOIN msg_payloads p ON p.id = j.payload_id AND p.created_at = j.created_at
WHERE j.id = $1;

-- name: DispatchJobAttemptInsert :exec
-- One row per delivery attempt. The schema column `status` stores the
-- attempt outcome (`SUCCESS` / `FAILURE`); the entity exposes a
-- derived `success` bool to match the legacy-platform wire shape.
INSERT INTO msg_dispatch_job_attempts
    (id, dispatch_job_id, attempt_number, status, response_code,
     response_body, error_message, error_type, duration_millis,
     attempted_at, completed_at, created_at, request_info)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: DispatchJobAttemptsByJob :many
SELECT attempt_number, attempted_at, completed_at, duration_millis,
       response_code, response_body, status, error_message, error_type,
       request_info
FROM msg_dispatch_job_attempts
WHERE dispatch_job_id = $1
-- Chronological, not by attempt_number: a requeue starts a new run whose
-- attempts are numbered from 1 again, so ordering by number interleaves
-- runs. attempted_at is the order an operator reads them in.
ORDER BY attempted_at ASC, attempt_number ASC;

-- name: DispatchJobFindByIDs :many
-- Batch load by id (write table), for the Resend operation, which reloads
-- multiple aggregates to reset via usecaseop.SaveAll. The same columns as
-- DispatchJobFindByID, payload join included; nothing here resolves an
-- offloaded payload. In id order, the order the primary-key scan returned
-- before the join: the persisters lock the rows in it, and a concurrent
-- scheduler claim must not meet them in another order (a deadlock).
SELECT j.id, j.external_id, j.source, j.kind, j.code, j.subject, j.event_id,
       j.correlation_id, j.metadata, j.target_url, j.protocol, j.payload,
       j.payload_content_type, j.data_only, j.service_account_id, j.client_id,
       j.subscription_id, j.mode, j.dispatch_pool_id, j.message_group, j.sequence,
       j.timeout_seconds, j.schema_id, j.status, j.max_retries, j.retry_strategy,
       j.scheduled_for, j.expires_at, j.attempt_count, j.last_attempt_at,
       j.completed_at, j.duration_millis, j.last_error, j.idempotency_key,
       j.queue, j.descriptor, j.connection_id, j.not_before, j.created_at, j.updated_at,
       j.payload_id, p.data AS payload_data, p.ref AS payload_ref, p.size AS payload_size
FROM msg_dispatch_jobs j
LEFT JOIN msg_payloads p ON p.id = j.payload_id AND p.created_at = j.created_at
WHERE j.id = ANY(sqlc.arg('ids')::text[])
ORDER BY j.id, j.created_at;

