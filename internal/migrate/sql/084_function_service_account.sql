-- +goose Up
-- The function's own webhook verifier identity: the service account whose
-- signing secret a `webhook`-auth endpoint of this function verifies
-- deliveries with. NULL (the default, and every existing row) keeps the old
-- behaviour: the oldest active service account linked to the function's
-- application. A bare reference by id, no FK, like application_id /
-- client_id on this table (an account deleted out from under a function
-- yields an empty secret and a logged reason, not a failed delete).
ALTER TABLE fng_functions ADD COLUMN IF NOT EXISTS service_account_id VARCHAR(17);

-- +goose Down
ALTER TABLE fng_functions DROP COLUMN IF EXISTS service_account_id;
