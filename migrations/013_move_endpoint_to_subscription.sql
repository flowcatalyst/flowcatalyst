-- 013: Move endpoint from connections to subscriptions
--
-- Connections become auth/pause groups only (service_account + status).
-- Subscriptions own their webhook endpoint URL directly.
-- This aligns with the SDK which sends `target` per subscription.

-- Add endpoint column to subscriptions (NOT NULL with empty default for existing rows)
ALTER TABLE msg_subscriptions ADD COLUMN IF NOT EXISTS endpoint VARCHAR(2048) NOT NULL DEFAULT '';

-- Copy endpoint from connection to subscription for any existing data
UPDATE msg_subscriptions s
SET endpoint = c.endpoint
FROM msg_connections c
WHERE s.connection_id = c.id
  AND s.endpoint = '';

-- Make connection_id optional (subscriptions can exist without a connection)
ALTER TABLE msg_subscriptions ALTER COLUMN connection_id DROP NOT NULL;

-- Drop endpoint from connections
ALTER TABLE msg_connections DROP COLUMN IF EXISTS endpoint;
