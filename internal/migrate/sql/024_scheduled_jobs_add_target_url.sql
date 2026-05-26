-- Add `target_url` to msg_scheduled_jobs.
--
-- The column was added to migration 021 after some dev DBs had already run
-- the original 021. CREATE TABLE IF NOT EXISTS is a no-op on those DBs
-- (table exists), and the tracker backfill marks 021 applied based on the
-- table's existence, so 021 will never run again. This additive migration
-- bridges the gap.
--
-- ADD COLUMN IF NOT EXISTS makes this safe on DBs where 021 already
-- includes target_url (no-op).

ALTER TABLE msg_scheduled_jobs
    ADD COLUMN IF NOT EXISTS target_url VARCHAR(500);
