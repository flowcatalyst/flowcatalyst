-- Queries for msg_dispatch_jobs + msg_dispatch_job_attempts. The
-- column set matches the post-019 (partitioned) schema. Composite PK
-- is (id, created_at); claim queries use FOR UPDATE SKIP LOCKED so
-- multiple scheduler nodes can run against the same DB without
-- contention.
--
-- FindWithFilters + DistinctValues stay hand-rolled in repository.go
-- (dynamic WHERE + dynamic column names) — sqlc can't generate those
-- without a query per filter combination.
--
-- InsertBatch also stays in repository.go via pgx.Batch — sqlc has no
-- batch wrapper for partial-failure-tolerant UNNEST inserts.

-- name: DispatchJobFindByID :one
SELECT id, external_id, source, kind, code, subject, event_id,
       correlation_id, metadata, target_url, protocol, payload,
       payload_content_type, data_only, service_account_id, client_id,
       subscription_id, mode, dispatch_pool_id, message_group, sequence,
       timeout_seconds, schema_id, status, max_retries, retry_strategy,
       scheduled_for, expires_at, attempt_count, last_attempt_at,
       completed_at, duration_millis, last_error, idempotency_key,
       created_at, updated_at
FROM msg_dispatch_jobs
WHERE id = $1;

-- name: DispatchJobInsert :exec
INSERT INTO msg_dispatch_jobs
    (id, external_id, source, kind, code, subject, event_id, correlation_id,
     metadata, target_url, protocol, payload, payload_content_type, data_only,
     service_account_id, client_id, subscription_id, mode, dispatch_pool_id,
     message_group, sequence, timeout_seconds, schema_id, status, max_retries,
     retry_strategy, scheduled_for, expires_at, attempt_count, last_attempt_at,
     completed_at, duration_millis, last_error, idempotency_key, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
        $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26,
        $27, $28, $29, $30, $31, $32, $33, $34, $35, $36);

-- name: DispatchJobMarkInProgress :exec
-- Status → PROCESSING. Stamps last_attempt_at. Called by the router
-- immediately before the first delivery attempt.
UPDATE msg_dispatch_jobs
   SET status = 'PROCESSING',
       last_attempt_at = $2,
       updated_at = $2
 WHERE id = $1;

-- name: DispatchJobMarkCompleted :exec
-- Status → COMPLETED. Stamps completed_at + duration_millis.
UPDATE msg_dispatch_jobs
   SET status = 'COMPLETED',
       completed_at = $2,
       duration_millis = $3,
       updated_at = $2
 WHERE id = $1;

-- name: DispatchJobMarkFailed :exec
-- Terminal failure. Stamps last_error + completed_at + duration_millis.
UPDATE msg_dispatch_jobs
   SET status = 'FAILED',
       completed_at = $2,
       duration_millis = $3,
       last_error = $4,
       updated_at = $2
 WHERE id = $1;

-- name: DispatchJobScheduleRetry :exec
-- Bumps attempt_count + stamps scheduled_for so the next poll picks
-- it up once due. Status stays PENDING.
UPDATE msg_dispatch_jobs
   SET attempt_count = attempt_count + 1,
       scheduled_for = $2,
       last_error = $3,
       last_attempt_at = NOW(),
       status = 'PENDING',
       updated_at = NOW()
 WHERE id = $1;

-- name: DispatchJobAttemptInsert :exec
-- One row per delivery attempt. The schema column `status` stores the
-- attempt outcome (`SUCCESS` / `FAILURE`); the entity exposes a
-- derived `success` bool to match the Rust wire shape.
INSERT INTO msg_dispatch_job_attempts
    (id, dispatch_job_id, attempt_number, status, response_code,
     response_body, error_message, error_type, duration_millis,
     attempted_at, completed_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: DispatchJobAttemptsByJob :many
SELECT attempt_number, attempted_at, completed_at, duration_millis,
       response_code, response_body, status, error_message, error_type
FROM msg_dispatch_job_attempts
WHERE dispatch_job_id = $1
ORDER BY attempt_number ASC;
