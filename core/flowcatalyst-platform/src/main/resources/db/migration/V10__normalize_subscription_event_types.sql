-- V10: Normalize subscription_event_types from JSONB to proper table
--
-- The subscriptions.event_types column stores subscribed event type codes as JSONB:
-- ["order:created", "order:updated", "payment:completed"]
--
-- This migration creates a proper junction table for better querying and joins.

-- =============================================================================
-- Create subscription_event_types table
-- =============================================================================

CREATE TABLE subscription_event_types (
    subscription_id VARCHAR(17) NOT NULL,
    event_type_code VARCHAR(200) NOT NULL,
    PRIMARY KEY (subscription_id, event_type_code)
);

-- Index for finding all subscriptions for a specific event type
CREATE INDEX idx_subscription_event_types_code ON subscription_event_types(event_type_code);

-- =============================================================================
-- Migrate existing JSONB data
-- =============================================================================

-- Insert rows from the existing JSONB column
-- Handle both string array format ["type1", "type2"] and object array format
INSERT INTO subscription_event_types (subscription_id, event_type_code)
SELECT
    s.id as subscription_id,
    CASE
        -- If it's a simple string, use it directly
        WHEN jsonb_typeof(et_elem) = 'string' THEN et_elem #>> '{}'
        -- If it's an object with eventTypeCode property, extract that
        WHEN jsonb_typeof(et_elem) = 'object' THEN et_elem->>'eventTypeCode'
        ELSE NULL
    END as event_type_code
FROM subscriptions s,
     jsonb_array_elements(COALESCE(s.event_types::jsonb, '[]'::jsonb)) as et_elem
WHERE s.event_types IS NOT NULL
  AND s.event_types != '[]'
  AND (
    (jsonb_typeof(et_elem) = 'string' AND et_elem #>> '{}' IS NOT NULL)
    OR
    (jsonb_typeof(et_elem) = 'object' AND et_elem->>'eventTypeCode' IS NOT NULL)
  );

-- =============================================================================
-- Verification query (run manually to check migration)
-- =============================================================================
-- SELECT s.id,
--        jsonb_array_length(COALESCE(s.event_types::jsonb, '[]')) as jsonb_count,
--        (SELECT COUNT(*) FROM subscription_event_types set WHERE set.subscription_id = s.id) as table_count
-- FROM subscriptions s
-- WHERE jsonb_array_length(COALESCE(s.event_types::jsonb, '[]')) !=
--       (SELECT COUNT(*) FROM subscription_event_types set WHERE set.subscription_id = s.id);
-- Should return 0 rows if migration was successful

-- =============================================================================
-- NOTE: The subscriptions.event_types column is NOT dropped yet.
-- It will be removed in a future migration (V12) after:
-- 1. Application code is updated to use the new table
-- 2. Validation period confirms data integrity
-- =============================================================================
