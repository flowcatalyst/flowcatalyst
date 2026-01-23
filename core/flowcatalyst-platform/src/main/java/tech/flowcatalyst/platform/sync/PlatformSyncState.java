package tech.flowcatalyst.platform.sync;

import jakarta.persistence.*;
import java.time.Instant;

/**
 * Tracks the sync state of platform registries (event types, subscriptions, etc).
 *
 * <p>Stores a hash of the code-defined content to detect when re-sync is needed.
 * Each registry (e.g., "platform-event-types") has its own row.
 */
@Entity
@Table(name = "platform_sync_state")
public class PlatformSyncState {

    @Id
    @Column(name = "registry_name", length = 100)
    public String registryName;

    @Column(name = "content_hash", length = 64, nullable = false)
    public String contentHash;

    @Column(name = "synced_at", nullable = false)
    public Instant syncedAt;

    @Column(name = "synced_by", length = 255)
    public String syncedBy;

    @Column(name = "items_synced")
    public Integer itemsSynced;

    public PlatformSyncState() {}

    public PlatformSyncState(String registryName, String contentHash, Instant syncedAt, String syncedBy, Integer itemsSynced) {
        this.registryName = registryName;
        this.contentHash = contentHash;
        this.syncedAt = syncedAt;
        this.syncedBy = syncedBy;
        this.itemsSynced = itemsSynced;
    }

    /**
     * Check if the stored hash matches the given hash.
     */
    public boolean matches(String hash) {
        return contentHash != null && contentHash.equals(hash);
    }
}
