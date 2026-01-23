-- Add client_scoped column to event_types and subscriptions tables
-- This column indicates whether the event type / subscription is scoped to a specific client context

-- ============================================================================
-- EVENT_TYPES: Add client_scoped column
-- ============================================================================

ALTER TABLE event_types ADD COLUMN client_scoped BOOLEAN NOT NULL DEFAULT false;

-- Add comment for documentation
COMMENT ON COLUMN event_types.client_scoped IS 'Whether events of this type are scoped to a specific client. Immutable after creation.';

-- ============================================================================
-- SUBSCRIPTIONS: Add client_scoped column
-- ============================================================================

ALTER TABLE subscriptions ADD COLUMN client_scoped BOOLEAN NOT NULL DEFAULT false;

-- For existing subscriptions, derive client_scoped from whether client_id is set
-- (If a subscription has a client_id, it's client-scoped)
UPDATE subscriptions SET client_scoped = true WHERE client_id IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN subscriptions.client_scoped IS 'Whether this subscription is scoped to clients. Immutable after creation. When true, client_id null means all clients.';
