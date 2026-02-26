-- Add application_id and client_id columns to aud_logs for application/client context.
-- These are nullable — existing rows and internal platform operations have no context.

ALTER TABLE "aud_logs"
  ADD COLUMN IF NOT EXISTS "application_id" varchar(17),
  ADD COLUMN IF NOT EXISTS "client_id" varchar(17);

CREATE INDEX IF NOT EXISTS "idx_aud_logs_application" ON "aud_logs" ("application_id");
CREATE INDEX IF NOT EXISTS "idx_aud_logs_client" ON "aud_logs" ("client_id");
