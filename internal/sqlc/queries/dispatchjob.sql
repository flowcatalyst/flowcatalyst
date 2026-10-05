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
SELECT id, external_id, source, kind, code, subject, event_id,
       correlation_id, metadata, target_url, protocol, payload,
       payload_content_type, data_only, service_account_id, client_id,
       subscription_id, mode, dispatch_pool_id, message_group, sequence,
       timeout_seconds, schema_id, status, max_retries, retry_strategy,
       scheduled_for, expires_at, attempt_count, last_attempt_at,
       completed_at, duration_millis, last_error, idempotency_key,
       queue, descriptor, created_at, updated_at
FROM msg_dispatch_jobs
WHERE id = $1;

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
-- multiple aggregates to reset via usecaseop.SaveAll.
SELECT id, external_id, source, kind, code, subject, event_id,
       correlation_id, metadata, target_url, protocol, payload,
       payload_content_type, data_only, service_account_id, client_id,
       subscription_id, mode, dispatch_pool_id, message_group, sequence,
       timeout_seconds, schema_id, status, max_retries, retry_strategy,
       scheduled_for, expires_at, attempt_count, last_attempt_at,
       completed_at, duration_millis, last_error, idempotency_key,
       queue, descriptor, created_at, updated_at
FROM msg_dispatch_jobs
WHERE id = ANY(sqlc.arg('ids')::text[]);

