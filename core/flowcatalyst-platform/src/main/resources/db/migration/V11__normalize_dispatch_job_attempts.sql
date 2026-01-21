-- V11: Normalize dispatch_job_attempts from JSONB to proper table
--
-- The dispatch_jobs.attempts column stores dispatch attempt history as JSONB:
-- [{"attemptNumber": 1, "status": "FAILED", "statusCode": 500, "errorMessage": "...", "durationMillis": 1234, "attemptedAt": "2024-..."}]
--
-- This migration creates a proper table for better querying and analytics.

-- =============================================================================
-- Create dispatch_job_attempts table
-- =============================================================================

CREATE TABLE dispatch_job_attempts (
    id VARCHAR(17) PRIMARY KEY,
    dispatch_job_id VARCHAR(13) NOT NULL,
    attempt_number INT NOT NULL,
    status VARCHAR(20) NOT NULL,
    status_code INT,
    error_message TEXT,
    duration_millis BIGINT,
    attempted_at TIMESTAMPTZ NOT NULL,
    UNIQUE(dispatch_job_id, attempt_number)
);

-- Index for finding attempts by job
CREATE INDEX idx_dispatch_job_attempts_job_id ON dispatch_job_attempts(dispatch_job_id);

-- Index for finding failed attempts (useful for retry analysis)
CREATE INDEX idx_dispatch_job_attempts_status ON dispatch_job_attempts(status);

-- Index for time-based queries
CREATE INDEX idx_dispatch_job_attempts_attempted_at ON dispatch_job_attempts(attempted_at);

-- =============================================================================
-- Migrate existing JSONB data
-- =============================================================================

-- Generate IDs for migrated records using a sequence or TSID function
-- Note: If gen_random_uuid() is available, we use it for simplicity
-- In production, you'd use the TsidGenerator equivalent

-- Handle attemptedAt as either epoch milliseconds (numeric) or ISO timestamp string
INSERT INTO dispatch_job_attempts (id, dispatch_job_id, attempt_number, status, status_code, error_message, duration_millis, attempted_at)
SELECT
    -- Generate a simple ID (prefix djat_ + random suffix)
    'djat_' || substring(md5(random()::text) for 13) as id,
    dj.id as dispatch_job_id,
    (attempt_elem->>'attemptNumber')::int as attempt_number,
    COALESCE(attempt_elem->>'status', 'UNKNOWN') as status,
    (attempt_elem->>'statusCode')::int as status_code,
    attempt_elem->>'errorMessage' as error_message,
    (attempt_elem->>'durationMillis')::bigint as duration_millis,
    COALESCE(
        CASE
            -- If it's a numeric value (epoch milliseconds), convert using to_timestamp
            WHEN attempt_elem->>'attemptedAt' ~ '^\d+\.?\d*$'
            THEN to_timestamp((attempt_elem->>'attemptedAt')::double precision / 1000)
            -- Otherwise try to parse as ISO timestamp
            ELSE (attempt_elem->>'attemptedAt')::timestamptz
        END,
        dj.created_at
    ) as attempted_at
FROM dispatch_jobs dj,
     jsonb_array_elements(COALESCE(dj.attempts::jsonb, '[]'::jsonb)) as attempt_elem
WHERE dj.attempts IS NOT NULL
  AND dj.attempts != '[]';

-- =============================================================================
-- Verification query (run manually to check migration)
-- =============================================================================
-- SELECT dj.id,
--        jsonb_array_length(COALESCE(dj.attempts::jsonb, '[]')) as jsonb_count,
--        (SELECT COUNT(*) FROM dispatch_job_attempts dja WHERE dja.dispatch_job_id = dj.id) as table_count
-- FROM dispatch_jobs dj
-- WHERE jsonb_array_length(COALESCE(dj.attempts::jsonb, '[]')) !=
--       (SELECT COUNT(*) FROM dispatch_job_attempts dja WHERE dja.dispatch_job_id = dj.id);
-- Should return 0 rows if migration was successful

-- =============================================================================
-- NOTE: The dispatch_jobs.attempts column is NOT dropped yet.
-- It will be removed in a future migration (V12) after:
-- 1. Application code is updated to use the new table
-- 2. Validation period confirms data integrity
-- =============================================================================
