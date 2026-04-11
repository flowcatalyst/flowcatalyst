-- Add projected_at columns directly to source tables so the stream processor
-- can read from msg_events / msg_dispatch_jobs instead of requiring separate
-- projection feed tables.

ALTER TABLE msg_events ADD COLUMN IF NOT EXISTS projected_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_msg_events_unprojected ON msg_events (created_at) WHERE projected_at IS NULL;

ALTER TABLE msg_dispatch_jobs ADD COLUMN IF NOT EXISTS projected_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_msg_dispatch_jobs_unprojected ON msg_dispatch_jobs (created_at) WHERE projected_at IS NULL;
