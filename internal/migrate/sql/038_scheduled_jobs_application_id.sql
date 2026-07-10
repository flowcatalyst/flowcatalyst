-- +goose Up
-- Scheduled jobs currently store only client_id (tenant scoping). Add
-- application_id (the registered Application axis, distinct from client) so
-- jobs synced via the SDK's `/api/applications/{appCode}/scheduled-jobs/sync`
-- endpoint — and jobs created directly with an explicit application — can be
-- filtered/displayed by application. Additive + nullable, no FK: mirrors
-- iam_service_accounts.application_id (migration 002), which also has no FK
-- back to app_applications.

ALTER TABLE msg_scheduled_jobs ADD COLUMN application_id VARCHAR(17);
CREATE INDEX IF NOT EXISTS idx_msg_scheduled_jobs_application_id
    ON msg_scheduled_jobs (application_id);
