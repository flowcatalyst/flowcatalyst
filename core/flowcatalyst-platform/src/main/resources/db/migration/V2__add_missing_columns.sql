-- V2: Add missing columns for entity compatibility
-- These columns exist in the entity models but were not in the initial schema

-- Add missing columns to dispatch_pools
ALTER TABLE dispatch_pools ADD COLUMN IF NOT EXISTS name VARCHAR(255);
ALTER TABLE dispatch_pools ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE dispatch_pools ADD COLUMN IF NOT EXISTS client_identifier VARCHAR(100);

-- Backfill name from code if null
UPDATE dispatch_pools SET name = code WHERE name IS NULL;

-- Add missing column to events
ALTER TABLE events ADD COLUMN IF NOT EXISTS spec_version VARCHAR(50);

-- Add missing columns to schemas
ALTER TABLE schemas ADD COLUMN IF NOT EXISTS name VARCHAR(255);
ALTER TABLE schemas ADD COLUMN IF NOT EXISTS description TEXT;

-- Add missing columns to dispatch_jobs_read
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS sequence INT DEFAULT 99;
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS timeout_seconds INT DEFAULT 30;
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS retry_strategy VARCHAR(50);
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS scheduled_for TIMESTAMPTZ;
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(100);
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS is_completed BOOLEAN;
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS is_terminal BOOLEAN;
ALTER TABLE dispatch_jobs_read ADD COLUMN IF NOT EXISTS projected_at TIMESTAMPTZ;
