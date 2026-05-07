-- Make rate_limit opt-in: a dispatch pool can run on concurrency only.
-- Drops the NOT NULL and DEFAULT 100 introduced in 004; existing rows keep
-- their stored rate_limit value, but new pools and updates may set NULL to
-- mean "no rate limit" (the message router already supports this).

ALTER TABLE msg_dispatch_pools
    ALTER COLUMN rate_limit DROP NOT NULL;

ALTER TABLE msg_dispatch_pools
    ALTER COLUMN rate_limit DROP DEFAULT;
