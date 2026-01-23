-- Platform sync state table for tracking code-to-database synchronization
-- Used by PlatformEventTypeSyncService to detect when platform registries need re-syncing

CREATE TABLE platform_sync_state (
    registry_name VARCHAR(100) PRIMARY KEY,
    content_hash VARCHAR(64) NOT NULL,
    synced_at TIMESTAMP WITH TIME ZONE NOT NULL,
    synced_by VARCHAR(255),
    items_synced INTEGER
);

-- Add comment for documentation
COMMENT ON TABLE platform_sync_state IS 'Tracks sync state of platform registries (event types, subscriptions, etc)';
COMMENT ON COLUMN platform_sync_state.registry_name IS 'Unique name of the registry (e.g., platform-event-types)';
COMMENT ON COLUMN platform_sync_state.content_hash IS 'SHA-256 hash of registry content for change detection';
COMMENT ON COLUMN platform_sync_state.synced_at IS 'When the last sync occurred';
COMMENT ON COLUMN platform_sync_state.synced_by IS 'Who/what triggered the sync (system, admin, etc)';
COMMENT ON COLUMN platform_sync_state.items_synced IS 'Number of items in the registry at sync time';
