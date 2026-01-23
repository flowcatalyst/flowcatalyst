package tech.flowcatalyst.platform.sync;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;

import java.util.Optional;

/**
 * Repository for PlatformSyncState entities.
 */
@ApplicationScoped
public class PlatformSyncStateRepository implements PanacheRepositoryBase<PlatformSyncState, String> {

    /**
     * Find sync state by registry name.
     */
    public Optional<PlatformSyncState> findByRegistryName(String registryName) {
        return findByIdOptional(registryName);
    }

    /**
     * Check if the stored hash matches the given hash for a registry.
     */
    public boolean hashMatches(String registryName, String hash) {
        return findByRegistryName(registryName)
            .map(state -> state.matches(hash))
            .orElse(false);
    }
}
