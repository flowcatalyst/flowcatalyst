package tech.flowcatalyst.platform.sync;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import org.jboss.logging.Logger;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.eventtype.EventTypeSource;
import tech.flowcatalyst.eventtype.EventTypeStatus;
import tech.flowcatalyst.platform.lock.DistributedLock;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.platform.sync.PlatformEventTypeRegistry.PlatformEventTypeDefinition;

import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.function.Function;
import java.util.stream.Collectors;

/**
 * Service for syncing platform event types from code to database.
 *
 * <p>On startup, compares the hash of code-defined event types with the stored hash.
 * If different, acquires a distributed lock and syncs the event types.
 *
 * <p>Sync logic:
 * <ul>
 *   <li>Creates new event types that don't exist</li>
 *   <li>Updates existing CODE-sourced event types if changed</li>
 *   <li>Does NOT delete event types no longer in the registry (soft removal)</li>
 *   <li>Does NOT modify API-sourced or UI-sourced event types</li>
 * </ul>
 */
@ApplicationScoped
public class PlatformEventTypeSyncService {

    private static final Logger LOG = Logger.getLogger(PlatformEventTypeSyncService.class);
    private static final String REGISTRY_NAME = "platform-event-types";
    private static final String LOCK_NAME = "platform-event-types-sync";
    private static final Duration LOCK_TIMEOUT = Duration.ofSeconds(30);

    @Inject
    PlatformEventTypeRegistry registry;

    @Inject
    EventTypeRepository eventTypeRepo;

    @Inject
    PlatformSyncStateRepository syncStateRepo;

    @Inject
    DistributedLock distributedLock;

    /**
     * Check if sync is needed and perform sync if necessary.
     *
     * @return SyncResult describing what happened
     */
    @Transactional
    public SyncResult syncIfNeeded() {
        String currentHash = registry.getContentHash();

        // Check if sync is needed
        Optional<PlatformSyncState> existingState = syncStateRepo.findByRegistryName(REGISTRY_NAME);
        if (existingState.isPresent() && existingState.get().matches(currentHash)) {
            LOG.debugf("Platform event types already synced (hash=%s)", currentHash.substring(0, 8));
            return new SyncResult(false, 0, 0, 0, "Already up to date");
        }

        // Acquire distributed lock before syncing
        return distributedLock.withLock(LOCK_NAME, LOCK_TIMEOUT, () -> {
            // Double-check after acquiring lock (another instance may have synced)
            Optional<PlatformSyncState> stateAfterLock = syncStateRepo.findByRegistryName(REGISTRY_NAME);
            if (stateAfterLock.isPresent() && stateAfterLock.get().matches(currentHash)) {
                LOG.debugf("Platform event types synced by another instance (hash=%s)", currentHash.substring(0, 8));
                return new SyncResult(false, 0, 0, 0, "Synced by another instance");
            }

            // Perform the sync
            return doSync(currentHash);
        }).orElseGet(() -> {
            LOG.warn("Could not acquire lock for platform event types sync");
            return new SyncResult(false, 0, 0, 0, "Could not acquire lock");
        });
    }

    /**
     * Force sync regardless of hash. Still acquires lock.
     */
    @Transactional
    public SyncResult forceSync() {
        String currentHash = registry.getContentHash();

        return distributedLock.withLock(LOCK_NAME, LOCK_TIMEOUT, () -> doSync(currentHash))
            .orElseGet(() -> {
                LOG.warn("Could not acquire lock for forced platform event types sync");
                return new SyncResult(false, 0, 0, 0, "Could not acquire lock");
            });
    }

    private SyncResult doSync(String currentHash) {
        LOG.infof("Syncing platform event types (hash=%s)...", currentHash.substring(0, 8));

        List<PlatformEventTypeDefinition> definitions = registry.getDefinitions();
        Instant now = Instant.now();

        // Get existing CODE-sourced event types by code
        Map<String, EventType> existingByCode = eventTypeRepo.listAll().stream()
            .filter(et -> et.source() == EventTypeSource.CODE)
            .collect(Collectors.toMap(EventType::code, Function.identity()));

        int created = 0;
        int updated = 0;

        for (PlatformEventTypeDefinition def : definitions) {
            EventType existing = existingByCode.get(def.code());

            if (existing == null) {
                // Check if there's an API or UI-sourced event type with this code
                Optional<EventType> anyExisting = eventTypeRepo.findByCode(def.code());
                if (anyExisting.isPresent()) {
                    LOG.debugf("Skipping %s - exists with source=%s", def.code(), anyExisting.get().source());
                    continue;
                }

                // Create new event type
                var segments = def.code().split(":");
                EventType newType = new EventType(
                    TsidGenerator.generate(EntityType.EVENT_TYPE),
                    def.code(),
                    def.name(),
                    def.description(),
                    List.of(),
                    EventTypeStatus.CURRENT,
                    EventTypeSource.CODE,
                    def.clientScoped(),
                    segments.length > 0 ? segments[0] : "",
                    segments.length > 1 ? segments[1] : "",
                    segments.length > 2 ? segments[2] : "",
                    now,
                    now
                );
                eventTypeRepo.persist(newType);
                created++;
                LOG.debugf("Created platform event type: %s", def.code());
            } else {
                // Update if changed
                if (!matches(existing, def)) {
                    EventType updatedType = new EventType(
                        existing.id(),
                        existing.code(),
                        def.name(),
                        def.description(),
                        existing.specVersions(),
                        existing.status(),
                        EventTypeSource.CODE,
                        def.clientScoped(),
                        existing.application(),
                        existing.subdomain(),
                        existing.aggregate(),
                        existing.createdAt(),
                        now
                    );
                    eventTypeRepo.update(updatedType);
                    updated++;
                    LOG.debugf("Updated platform event type: %s", def.code());
                }
            }
        }

        // Update sync state
        PlatformSyncState state = syncStateRepo.findByRegistryName(REGISTRY_NAME)
            .orElse(new PlatformSyncState());
        state.registryName = REGISTRY_NAME;
        state.contentHash = currentHash;
        state.syncedAt = now;
        state.syncedBy = "system";
        state.itemsSynced = definitions.size();
        syncStateRepo.persist(state);

        LOG.infof("Platform event types sync complete: %d created, %d updated, %d total",
            created, updated, definitions.size());

        return new SyncResult(true, created, updated, definitions.size(), "Sync completed");
    }

    private boolean matches(EventType existing, PlatformEventTypeDefinition def) {
        return existing.name().equals(def.name())
            && java.util.Objects.equals(existing.description(), def.description())
            && existing.clientScoped() == def.clientScoped();
    }

    /**
     * Result of a sync operation.
     */
    public record SyncResult(
        boolean performed,
        int created,
        int updated,
        int total,
        String message
    ) {}
}
