-- +goose Up
-- Optional link from a field identity to one platform user
-- (docs/field-native-session-spec.md section 5): audit correlation only, it
-- grants nothing on either side. No foreign key, as elsewhere in the module:
-- deleting the principal leaves the link, which the console shows as
-- unresolvable. A person has at most one field identity per client; one
-- platform user may be linked in several clients (NULLs never collide).
-- Re-runnable; the column is nullable (metadata-only), and field_identities
-- is small (docs/large-table-schema-changes.md).
ALTER TABLE field_identities ADD COLUMN IF NOT EXISTS linked_principal_id VARCHAR(17);
CREATE UNIQUE INDEX IF NOT EXISTS uq_field_identities_linked_principal
    ON field_identities (client_id, linked_principal_id);

-- +goose Down
DROP INDEX IF EXISTS uq_field_identities_linked_principal;
ALTER TABLE field_identities DROP COLUMN IF EXISTS linked_principal_id;
