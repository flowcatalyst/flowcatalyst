-- +goose Up
-- Persist-boundary columns for fields the entities and API DTOs already
-- carry but the schema silently dropped (found by the operation-test
-- campaign). All additive + nullable: the legacy-platform twin's explicit column
-- lists ignore them, and its rows simply read back NULL.

ALTER TABLE msg_event_types ADD COLUMN IF NOT EXISTS created_by VARCHAR(17);
ALTER TABLE msg_subscriptions ADD COLUMN IF NOT EXISTS created_by VARCHAR(17);

ALTER TABLE iam_service_accounts ADD COLUMN IF NOT EXISTS scope VARCHAR(20);
ALTER TABLE iam_service_accounts ADD COLUMN IF NOT EXISTS client_ids TEXT[];

ALTER TABLE oauth_idp_role_mappings ADD COLUMN IF NOT EXISTS idp_type VARCHAR(50);
