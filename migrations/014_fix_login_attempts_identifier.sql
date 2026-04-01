-- 014: Fix login_attempts identifier column length
-- The column was created as VARCHAR(20) but needs to hold client IDs (up to 50 chars)
ALTER TABLE iam_login_attempts ALTER COLUMN identifier TYPE VARCHAR(255);
