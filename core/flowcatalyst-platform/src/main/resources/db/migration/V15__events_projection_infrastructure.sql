-- V15: Events projection infrastructure
--
-- Adds projected column for tracking which events have been projected to events_read.

-- =============================================================================
-- 1. Drop unused index on events table
-- =============================================================================
-- deduplication_id index is not used (we don't deduplicate on read path)
DROP INDEX IF EXISTS idx_events_deduplication_id;

-- =============================================================================
-- 2. Add projected column
-- =============================================================================
ALTER TABLE events ADD COLUMN projected BOOLEAN NOT NULL DEFAULT false;

-- =============================================================================
-- 3. Composite index for efficient polling
-- =============================================================================
-- Supports: SELECT * FROM events WHERE projected = false ORDER BY time LIMIT 100
-- Composite (projected, time) allows index-only scan for the WHERE + ORDER BY
CREATE INDEX idx_events_projected_time ON events(projected, time);

COMMENT ON COLUMN events.projected IS 'Whether this event has been projected to events_read';
