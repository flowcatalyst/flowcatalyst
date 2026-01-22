-- Add source field to event_types table for SDK sync support
-- Source indicates how the event type was created: API (SDK), UI, or CODE

-- Add source column with default 'UI' for existing records
ALTER TABLE event_types
ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'UI';

-- Add SDK source to subscription_source enum values (extend existing enum)
-- Note: Subscriptions already have a source column with API/UI values
-- SDK sync will use API source for subscriptions

-- Create index for efficient filtering by source
CREATE INDEX idx_event_types_source ON event_types(source);

-- Create composite index for efficient application + source filtering
CREATE INDEX idx_event_types_code_source ON event_types(code, source);
