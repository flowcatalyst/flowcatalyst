package tech.flowcatalyst.eventtype.operations.synceventtypes;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.eventtype.EventTypeSource;
import tech.flowcatalyst.eventtype.events.EventTypesSynced;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.time.Instant;
import java.util.*;

/**
 * Use case for syncing event types from an external application (SDK).
 *
 * <p>Event types are synced based on their code prefix (application code).
 * Only API-sourced event types can be modified via sync.
 *
 * <p>Note: A registered Application entity is NOT required. Event types can
 * be synced for modules/prefixes that are not registered applications.
 */
@ApplicationScoped
public class SyncEventTypesUseCase implements UseCase<SyncEventTypesCommand, EventTypesSynced> {

    @Inject
    EventTypeRepository eventTypeRepo;

    @Inject
    UnitOfWork unitOfWork;

    @Override
    public boolean authorizeResource(SyncEventTypesCommand command, ExecutionContext context) {
        var authz = context.authz();
        if (authz == null) return true;
        if (command.applicationCode() == null || command.applicationCode().isBlank()) return true;
        return authz.canAccessApplicationByCode(command.applicationCode());
    }

    @Transactional
    @Override
    public Result<EventTypesSynced> doExecute(SyncEventTypesCommand command, ExecutionContext context) {
        if (command.applicationCode() == null || command.applicationCode().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "APPLICATION_CODE_REQUIRED",
                "Application code is required",
                Map.of()
            ));
        }

        String codePrefix = command.applicationCode() + ":";

        Set<String> syncedCodes = new HashSet<>();
        int created = 0;
        int updated = 0;
        int deleted = 0;

        for (SyncEventTypesCommand.SyncEventTypeItem item : command.eventTypes()) {
            // Build full code from segments
            String fullCode = item.buildCode(command.applicationCode());
            syncedCodes.add(fullCode);

            Optional<EventType> existingOpt = eventTypeRepo.findByCode(fullCode);

            if (existingOpt.isPresent()) {
                EventType existing = existingOpt.get();
                // Only update API-sourced event types
                if (existing.source() == EventTypeSource.API) {
                    EventType updatedType = existing.toBuilder()
                        .name(item.name() != null ? item.name() : existing.name())
                        .description(item.description())
                        .updatedAt(Instant.now())
                        .build();
                    eventTypeRepo.update(updatedType);
                    updated++;
                }
                // Don't update UI-sourced event types from SDK sync
            } else {
                // Create new API-sourced event type (default to non-client-scoped for SDK sync)
                boolean clientScoped = item.clientScoped() != null ? item.clientScoped() : false;
                EventType newType = EventType.createFromApi(fullCode, item.name(), clientScoped)
                    .description(item.description())
                    .build();
                eventTypeRepo.persist(newType);
                created++;
            }
        }

        if (command.removeUnlisted()) {
            // Remove API-sourced event types that weren't in the sync list
            List<EventType> existingTypes = eventTypeRepo.findByCodePrefix(codePrefix);
            for (EventType existing : existingTypes) {
                if (existing.source() == EventTypeSource.API && !syncedCodes.contains(existing.code())) {
                    eventTypeRepo.delete(existing);
                    deleted++;
                }
            }
        }

        // Create domain event
        EventTypesSynced event = EventTypesSynced.fromContext(context)
            .applicationCode(command.applicationCode())
            .eventTypesCreated(created)
            .eventTypesUpdated(updated)
            .eventTypesDeleted(deleted)
            .syncedEventTypeCodes(new ArrayList<>(syncedCodes))
            .build();

        // Commit - no entity to persist (event types already persisted via repository)
        return unitOfWork.commitAll(List.of(), event, command);
    }
}
