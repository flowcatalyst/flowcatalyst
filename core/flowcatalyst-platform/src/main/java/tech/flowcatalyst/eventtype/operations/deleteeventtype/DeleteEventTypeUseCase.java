package tech.flowcatalyst.eventtype.operations.deleteeventtype;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.*;
import tech.flowcatalyst.eventtype.events.EventTypeDeleted;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for deleting an EventType.
 */
@ApplicationScoped
public class DeleteEventTypeUseCase implements UseCase<DeleteEventTypeCommand, EventTypeDeleted> {

    @Inject
    EventTypeRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    @Override
    public boolean authorizeResource(DeleteEventTypeCommand command, ExecutionContext context) {
        return true;
    }

    @Override
    public Result<EventTypeDeleted> doExecute(
            DeleteEventTypeCommand command,
            ExecutionContext context
    ) {
        // Load aggregate
        EventType eventType = repo.findByIdOptional(command.eventTypeId())
            .orElse(null);

        if (eventType == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "EVENT_TYPE_NOT_FOUND",
                "Event type not found",
                Map.of("eventTypeId", command.eventTypeId())
            ));
        }

        // Authorization check: can principal manage event types with this prefix?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canAccessResourceWithPrefix(eventType.code())) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to delete this event type",
                Map.of("eventTypeId", command.eventTypeId(), "code", eventType.code())
            ));
        }

        // Can delete if archived
        boolean canDelete = eventType.status() == EventTypeStatus.ARCHIVE;

        // Can delete if CURRENT with all schemas in FINALISING (never finalized)
        if (!canDelete && eventType.status() == EventTypeStatus.CURRENT && eventType.allVersionsFinalising()) {
            canDelete = true;
        }

        if (!canDelete) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CANNOT_DELETE",
                "Cannot delete event type. Must be ARCHIVE status, or CURRENT with all schemas in FINALISING status.",
                Map.of(
                    "eventTypeId", command.eventTypeId(),
                    "status", eventType.status()
                )
            ));
        }

        // Create domain event (before deletion so we have access to the entity)
        EventTypeDeleted event = EventTypeDeleted.fromContext(context)
            .eventTypeId(eventType.id())
            .code(eventType.code())
            .build();

        // Delete via commitDelete
        return unitOfWork.commitDelete(eventType, event, command);
    }
}
