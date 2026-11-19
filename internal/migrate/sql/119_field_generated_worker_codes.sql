-- +goose Up
-- Generated worker codes (docs/field-worker-identity-design.md section 4.4):
-- a field worker created without a code gets one the platform draws (three
-- letters, then field_policies.worker_code_digits digits).
--
-- worker_code_digits: the client's setting, 3 to 6 (validated by the policy
-- operation). It applies to codes generated afterwards only.
--
-- idempotency_key: the caller's key of a creation without a code, unique
-- within the client, so repeating the creation answers the worker it made
-- instead of making a second one. NULL for every existing worker and for a
-- worker created with a code (NULLs never collide).
--
-- Cost: a constant default and a nullable column are catalogue changes only
-- (no rewrite). The unique index is built on field_identities, which is small
-- (docs/large-table-schema-changes.md). lock_timeout makes the migration fail
-- fast rather than queue behind a long query. Re-runnable.
SET LOCAL lock_timeout = '5s';

ALTER TABLE field_policies
    ADD COLUMN IF NOT EXISTS worker_code_digits INTEGER NOT NULL DEFAULT 3;

ALTER TABLE field_identities
    ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(100);
CREATE UNIQUE INDEX IF NOT EXISTS uq_field_identities_idempotency_key
    ON field_identities (client_id, idempotency_key) WHERE idempotency_key IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS uq_field_identities_idempotency_key;
ALTER TABLE field_identities DROP COLUMN IF EXISTS idempotency_key;
ALTER TABLE field_policies DROP COLUMN IF EXISTS worker_code_digits;
