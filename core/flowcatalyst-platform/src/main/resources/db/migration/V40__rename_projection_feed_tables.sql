-- V40: Rename outbox/changes tables to projection feed tables
--
-- Clarifies that these tables are CQRS projection feeds (not integration outboxes).
-- The integration outbox (outbox_messages) is a separate concept used by SDK clients.

-- Rename event_outbox -> event_projection_feed
ALTER TABLE IF EXISTS event_outbox RENAME TO event_projection_feed;

-- Rename dispatch_job_changes -> dispatch_job_projection_feed
ALTER TABLE IF EXISTS dispatch_job_changes RENAME TO dispatch_job_projection_feed;

-- Rename indexes for event_projection_feed
ALTER INDEX IF EXISTS idx_event_outbox_unprocessed RENAME TO idx_event_projection_feed_unprocessed;
ALTER INDEX IF EXISTS idx_event_outbox_in_progress RENAME TO idx_event_projection_feed_in_progress;

-- Rename indexes for dispatch_job_projection_feed
ALTER INDEX IF EXISTS idx_dispatch_job_changes_unprojected RENAME TO idx_dj_projection_feed_unprojected;
ALTER INDEX IF EXISTS idx_dispatch_job_changes_job_id RENAME TO idx_dj_projection_feed_job_id;

-- Update comments
COMMENT ON TABLE event_projection_feed IS 'Projection feed for CQRS projection of events to events_read. Full event payload captured at write time.';
COMMENT ON TABLE dispatch_job_projection_feed IS 'Projection feed for CQRS projection of dispatch_jobs to dispatch_jobs_read. Uses dispatch_job_id as message group for sequencing.';
