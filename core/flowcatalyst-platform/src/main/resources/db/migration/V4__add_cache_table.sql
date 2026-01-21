-- V4: Add cache table for database-backed caching
-- Provides a simple cache alternative to Redis when distributed caching isn't needed

CREATE TABLE IF NOT EXISTS cache_entries (
    cache_name VARCHAR(100) NOT NULL,
    cache_key VARCHAR(500) NOT NULL,
    cache_value TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (cache_name, cache_key)
);

-- Index for cleanup of expired entries
CREATE INDEX idx_cache_entries_expires_at ON cache_entries(expires_at);

-- Index for invalidating all entries in a cache
CREATE INDEX idx_cache_entries_cache_name ON cache_entries(cache_name);

COMMENT ON TABLE cache_entries IS 'Database-backed cache for authorization and other frequently accessed data';
COMMENT ON COLUMN cache_entries.cache_name IS 'Cache namespace (e.g., principals, roles)';
COMMENT ON COLUMN cache_entries.cache_key IS 'Cache key within namespace';
COMMENT ON COLUMN cache_entries.cache_value IS 'Cached value as JSON string';
COMMENT ON COLUMN cache_entries.expires_at IS 'When this entry expires';
