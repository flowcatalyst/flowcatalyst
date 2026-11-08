-- +goose Up
-- Optional contact number for a field worker, stored as entered. Re-runnable;
-- the column is nullable (metadata-only), and field_identities is small
-- (docs/large-table-schema-changes.md).
ALTER TABLE field_identities ADD COLUMN IF NOT EXISTS phone VARCHAR(32);

-- +goose Down
ALTER TABLE field_identities DROP COLUMN IF EXISTS phone;
