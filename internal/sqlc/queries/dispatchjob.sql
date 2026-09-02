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
-- These status flips all carry `created_at = $N` alongside the id: the
-- table is partitioned by created_at, and without it every statement
-- probes every partition instead of pruning to the row's own.
UPDATE msg_dispatch_jobs
   SET status = 'PROCESSING',
       last_attempt_at = $2,
       updated_at = $2
 WHERE id = $1
   AND created_at = $3;

-- name: DispatchJobMarkCompleted :exec
-- Status → COMPLETED. Stamps completed_at + duration_millis.
UPDATE msg_dispatch_jobs
   SET status = 'COMPLETED',
       completed_at = $2,
       duration_millis = $3,
       updated_at = $2
 WHERE id = $1
   AND created_at = $4;

-- name: DispatchJobMarkFailed :exec
-- Terminal failure. Stamps last_error + completed_at + duration_millis.
UPDATE msg_dispatch_jobs
   SET status = 'FAILED',
       completed_at = $2,
       duration_millis = $3,
       last_error = $4,
       updated_at = $2
 WHERE id = $1
   AND created_at = $5;

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
 WHERE id = $1
   AND created_at = $4;

-- name: DispatchJobAttemptInsert :exec
-- One row per delivery attempt. The schema column `status` stores the
-- attempt outcome (`SUCCESS` / `FAILURE`); the entity exposes a
-- derived `success` bool to match the legacy-platform wire shape.
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

-- Queries below back T3/A-01 (BLOCK_ON_ERROR group recovery): the use-case
-- envelope ops (cancel/complete/resend, internal/platform/dispatchjob/operations),
-- the router-settled hook (internal/platform/dispatchjob/settled), and the
-- stranded-sibling reaper (internal/platform/dispatchjob/reaper.go).

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
       created_at, updated_at
FROM msg_dispatch_jobs
WHERE id = ANY(sqlc.arg('ids')::text[]);

-- name: DispatchJobPersist :exec
-- Mutable-field update for human-initiated status overrides that go through
-- the use-case envelope (cancel/complete/resend): scoped to the fields
-- those operations ever change. payload/metadata/target_url/etc are
-- write-once at ingest (DispatchJobInsert/InsertBatch) and never revisited
-- by this path. created_at carries alongside id for partition pruning, like
-- every other status-flip query in this file.
UPDATE msg_dispatch_jobs
   SET status = $2,
       attempt_count = $3,
       scheduled_for = $4,
       completed_at = $5,
       duration_millis = $6,
       last_error = $7,
       updated_at = $8
 WHERE id = $1
   AND created_at = $9;

-- name: DispatchJobDelete :exec
-- Satisfies usecasepgx.Persist[DispatchJob], which requires both Persist
-- and Delete. No operation in this module deletes a dispatch job today
-- (Cancel/Complete/Resend all use Save/SaveAll), so this exists purely for
-- interface conformance — but it's a real delete, not a stub, in case that
-- ever changes.
DELETE FROM msg_dispatch_jobs WHERE id = $1 AND created_at = $2;

-- name: DispatchJobSettleAcked :many
-- The router→platform settled-message hook: resets the given ids to
-- PENDING, recording the reason in last_error. Scoped to QUEUED/PROCESSING
-- so a row a concurrent path already advanced (to a terminal status, or
-- already back to PENDING) is left alone — idempotent, so a duplicate hook
-- call is harmless. No created_at is available (the router only knows job
-- ids), so this scans by id across partitions — the same accepted exception
-- the pre-existing operator Requeue-turned-Resend path already relies on.
UPDATE msg_dispatch_jobs
   SET status = 'PENDING',
       scheduled_for = NULL,
       last_error = sqlc.arg('reason')::text,
       updated_at = NOW()
 WHERE id = ANY(sqlc.arg('ids')::text[])
   AND status IN ('QUEUED', 'PROCESSING')
RETURNING id;

-- name: DispatchJobSweepStrandedSiblings :many
-- The reaper backstop (reaper.go): resets to PENDING any QUEUED/PROCESSING
-- job whose message group is headed by a terminally FAILED job under
-- BLOCK_ON_ERROR — i.e. rows the settled-message hook should have caught
-- but didn't (a dropped call, or a router crash between ACK and the hook).
-- A PROCESSING row updated more recently than sqlc.arg('live_before') is
-- presumed to be a genuine in-flight delivery and is left alone; QUEUED
-- rows have no such window (see reaper.go for why). Idempotent — a row
-- already reset by the hook no longer matches status IN (...) and is
-- skipped on the next sweep.
WITH stranded AS (
    SELECT s.id, s.created_at
      FROM msg_dispatch_jobs s
      JOIN msg_dispatch_jobs h
        ON h.message_group = s.message_group
       -- Both terminal-failure statuses, matching dispatchjob.GroupHoldingStatusSQL
       -- exactly. 'ERROR' is the legacy value that predates the current status
       -- set; the poller still treats it as holding its group, so a sweep that
       -- recognised only 'FAILED' would leave siblings behind an ERROR head held
       -- at claim time but never reset here — stranded permanently, which is the
       -- failure this reaper exists to prevent.
       AND h.status IN ('FAILED', 'ERROR')
       AND (h.sequence, h.created_at, h.id) < (s.sequence, s.created_at, s.id)
     WHERE s.mode = 'BLOCK_ON_ERROR'
       AND s.message_group IS NOT NULL
       AND s.status IN ('QUEUED', 'PROCESSING')
       AND (s.status <> 'PROCESSING' OR s.updated_at < sqlc.arg('live_before')::timestamptz)
)
UPDATE msg_dispatch_jobs j
   SET status = 'PENDING',
       scheduled_for = NULL,
       last_error = sqlc.arg('reason')::text,
       updated_at = NOW()
  FROM stranded st
 WHERE j.id = st.id AND j.created_at = st.created_at
RETURNING j.id;
