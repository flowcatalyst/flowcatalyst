package tech.flowcatalyst.eventtype.operations.archiveeventtype;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.*;
import tech.flowcatalyst.eventtype.events.EventTypeArchived;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for archiving an EventType.
 */
@ApplicationScoped
public class ArchiveEventTypeUseCase {

    @Inject
    EventTypeRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<EventTypeArchived> execute(
            ArchiveEventTypeCommand command,
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

        // Business rule: already archived
        if (eventType.status() == EventTypeStatus.ARCHIVE) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "ALREADY_ARCHIVED",
                "Event type is already archived",
                Map.of("eventTypeId", command.eventTypeId())
            ));
        }

        // Business rule: all versions must be deprecated
        if (!eventType.allVersionsDeprecated()) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "VERSIONS_NOT_DEPRECATED",
                "Cannot archive event type. All spec versions must be DEPRECATED first.",
                Map.of("eventTypeId", command.eventTypeId())
            ));
        }

        // Archive the event type immutably
        EventType updated = eventType.withStatus(EventTypeStatus.ARCHIVE);

        // Create domain event
        EventTypeArchived event = EventTypeArchived.fromContext(context)
            .eventTypeId(updated.id())
            .code(updated.code())
            .build();

        return unitOfWork.commit(updated, event, command);
    }
}
