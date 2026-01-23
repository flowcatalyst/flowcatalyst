-- V26: Add missing columns to subscription_event_types table

-- Add id column as primary key
ALTER TABLE subscription_event_types DROP CONSTRAINT subscription_event_types_pkey;
ALTER TABLE subscription_event_types ADD COLUMN id BIGSERIAL PRIMARY KEY;

-- Add event_type_id column
ALTER TABLE subscription_event_types ADD COLUMN event_type_id VARCHAR(17);

-- Add spec_version column
ALTER TABLE subscription_event_types ADD COLUMN spec_version VARCHAR(50);

-- Re-add unique constraint on (subscription_id, event_type_code)
CREATE UNIQUE INDEX idx_subscription_event_types_unique ON subscription_event_types(subscription_id, event_type_code);
