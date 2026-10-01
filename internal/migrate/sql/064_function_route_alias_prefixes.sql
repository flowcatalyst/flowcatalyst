-- +goose Up
-- Alias prefixes on a public route: a route may opt into serving named
-- aliases from prefixed hostnames, so `qa-myapp.acme.com` serves the `qa`
-- alias of the function that owns the route on `myapp.acme.com`. Empty (the
-- default) means exact-host matching only — behaviour before this column.
-- Derived hostnames are not stored; the runner derives them at match time.
ALTER TABLE fng_routes ADD COLUMN IF NOT EXISTS alias_prefixes TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE fng_routes DROP COLUMN IF EXISTS alias_prefixes;
