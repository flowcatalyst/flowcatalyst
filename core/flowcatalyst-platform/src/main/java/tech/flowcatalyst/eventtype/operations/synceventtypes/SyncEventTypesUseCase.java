package tech.flowcatalyst.eventtype.operations.synceventtypes;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.eventtype.EventTypeSource;
import tech.flowcatalyst.eventtype.events.EventTypesSynced;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.time.Instant;
import java.util.*;

/**
 * Use case for syncing event types from an external application (SDK).
 *
 * <p>Event types are synced based on their code prefix (application code).
 * Only API-sourced event types can be modified via sync.
 */
@ApplicationScoped
public class SyncEventTypesUseCase {

    @Inject
    EventTypeRepository eventTypeRepo;

    @Inject
    ApplicationRepository appRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<EventTypesSynced> execute(SyncEventTypesCommand command, ExecutionContext context) {
        if (command.applicationCode() == null || command.applicationCode().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "APPLICATION_CODE_REQUIRED",
                "Application code is required",
                Map.of()
            ));
        }

        // Look up application
        Application app = appRepo.findByCode(command.applicationCode()).orElse(null);
        if (app == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "APPLICATION_NOT_FOUND",
                "Application not found: " + command.applicationCode(),
                Map.of("applicationCode", command.applicationCode())
            ));
        }

        // Authorization check: can principal manage this application?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canManageApplication(app.id)) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to sync event types for this application",
                Map.of("applicationCode", command.applicationCode())
            ));
        }

        String codePrefix = command.applicationCode() + ":";
        Set<String> syncedCodes = new HashSet<>();
        int created = 0;
        int updated = 0;
        int deleted = 0;

        for (SyncEventTypesCommand.SyncEventTypeItem item : command.eventTypes()) {
            // Build full code with application prefix
            String fullCode = codePrefix + item.code();
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
                // Create new API-sourced event type
                EventType newType = EventType.createFromApi(fullCode, item.name())
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

        // Commit atomically with the app as the entity for the event
        return unitOfWork.commit(app, event, command);
    }
}
