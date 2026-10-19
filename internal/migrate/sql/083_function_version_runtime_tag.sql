-- +goose Up
-- Runtimes beyond the built-in 'wasm' and 'js' (a separately installed runner
-- advertises its own, e.g. 'jvm'): fng_versions.runtime becomes a runtime TAG
-- (lower-case, a letter first, 2-16 characters; the same pattern as
-- internal/functions/runtimes.ValidTag) instead of a two-value list, so adding
-- a runtime needs no migration. Which tags may be PUBLISHED is still decided in
-- code (publish validates the runtime against the describe readers it has).
ALTER TABLE fng_versions DROP CONSTRAINT IF EXISTS chk_fng_versions_runtime;
ALTER TABLE fng_versions
    ADD CONSTRAINT chk_fng_versions_runtime
    CHECK (runtime ~ '^[a-z][a-z0-9]{1,15}$');

-- +goose Down
ALTER TABLE fng_versions DROP CONSTRAINT IF EXISTS chk_fng_versions_runtime;
ALTER TABLE fng_versions
    ADD CONSTRAINT chk_fng_versions_runtime
    CHECK (runtime IN ('wasm', 'js'));
