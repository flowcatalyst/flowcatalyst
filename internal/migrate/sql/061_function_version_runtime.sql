-- +goose Up
-- Function versions declare which runtime they run on
-- (docs/function-runner-plan.md §9, WP4 of the phase-1 work packages,
-- §12.1): "wasm" (an ABI v1 module, compiled as is) or "js" (a bundled
-- script run on the shared JS engine, internal/functions/runtimes). Every
-- version published before this migration is a wasm module, hence the
-- DEFAULT — no backfill needed.
ALTER TABLE fng_versions ADD COLUMN IF NOT EXISTS runtime TEXT NOT NULL DEFAULT 'wasm';
ALTER TABLE fng_versions DROP CONSTRAINT IF EXISTS chk_fng_versions_runtime;
ALTER TABLE fng_versions
    ADD CONSTRAINT chk_fng_versions_runtime
    CHECK (runtime IN ('wasm', 'js'));

-- +goose Down
ALTER TABLE fng_versions DROP CONSTRAINT IF EXISTS chk_fng_versions_runtime;
ALTER TABLE fng_versions DROP COLUMN IF EXISTS runtime;
