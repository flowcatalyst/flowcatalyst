-- Optimize Event and DispatchJob ID columns from VARCHAR(17) to VARCHAR(13)
-- These entities no longer use TSID prefixes (evn_, djb_) for performance.
-- Untyped TSIDs are 13 characters (Crockford Base32).
-- Other entity IDs (clients, subscriptions, etc.) retain their 17-char prefixed format.

-- msg_events
ALTER TABLE msg_events ALTER COLUMN id TYPE VARCHAR(13);

-- msg_events_read
ALTER TABLE msg_events_read ALTER COLUMN id TYPE VARCHAR(13);

-- msg_dispatch_jobs (id and event_id FK)
ALTER TABLE msg_dispatch_jobs ALTER COLUMN id TYPE VARCHAR(13);
ALTER TABLE msg_dispatch_jobs ALTER COLUMN event_id TYPE VARCHAR(13);

-- msg_dispatch_jobs_read (id and event_id FK)
ALTER TABLE msg_dispatch_jobs_read ALTER COLUMN id TYPE VARCHAR(13);
ALTER TABLE msg_dispatch_jobs_read ALTER COLUMN event_id TYPE VARCHAR(13);

-- msg_dispatch_job_attempts (id and dispatch_job_id FK)
ALTER TABLE msg_dispatch_job_attempts ALTER COLUMN id TYPE VARCHAR(13);
ALTER TABLE msg_dispatch_job_attempts ALTER COLUMN dispatch_job_id TYPE VARCHAR(13);

-- msg_event_projection_feed (event_id FK)
ALTER TABLE msg_event_projection_feed ALTER COLUMN event_id TYPE VARCHAR(13);

-- msg_dispatch_job_projection_feed (dispatch_job_id FK)
ALTER TABLE msg_dispatch_job_projection_feed ALTER COLUMN dispatch_job_id TYPE VARCHAR(13);
