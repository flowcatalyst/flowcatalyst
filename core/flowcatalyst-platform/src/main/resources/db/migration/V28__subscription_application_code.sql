-- Add application_code column to subscriptions for filtering by application/module
-- This stores which application/module owns the subscription

ALTER TABLE subscriptions ADD COLUMN application_code VARCHAR(100);

-- Create index for efficient filtering by application
CREATE INDEX idx_subscriptions_application_code ON subscriptions(application_code);

-- Backfill existing subscriptions: remove client_identifier as it's redundant with client_id
-- Note: client_identifier was denormalized from clients table but client_id already references it
