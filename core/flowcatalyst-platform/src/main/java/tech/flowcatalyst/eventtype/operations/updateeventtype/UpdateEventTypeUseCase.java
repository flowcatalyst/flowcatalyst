package tech.flowcatalyst.eventtype.operations.updateeventtype;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.eventtype.events.EventTypeUpdated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for updating an EventType's metadata.
 */
@ApplicationScoped
public class UpdateEventTypeUseCase implements UseCase<UpdateEventTypeCommand, EventTypeUpdated> {

    private static final int MAX_NAME_LENGTH = 100;
    private static final int MAX_DESCRIPTION_LENGTH = 255;

    @Inject
    EventTypeRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    @Override
    public boolean authorizeResource(UpdateEventTypeCommand command, ExecutionContext context) {
        var authz = context.authz();
        if (authz == null) return true;
        return repo.findByIdOptional(command.eventTypeId())
            .map(et -> authz.canAccessResourceWithPrefix(et.code()))
            .orElse(true);
    }

    @Override
    public Result<EventTypeUpdated> doExecute(
            UpdateEventTypeCommand command,
            ExecutionContext context
    ) {
        // Load aggregate first (needed for authorization check)
        EventType eventType = repo.findByIdOptional(command.eventTypeId())
            .orElse(null);

        if (eventType == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "EVENT_TYPE_NOT_FOUND",
                "Event type not found",
                Map.of("eventTypeId", command.eventTypeId())
            ));
        }

        // Validation: at least one field to update
        if (command.name() == null && command.description() == null) {
            return Result.failure(new UseCaseError.ValidationError(
                "NO_CHANGES",
                "At least one field (name or description) must be provided",
                Map.of()
            ));
        }

        // Validation: name not blank if provided
        if (command.name() != null && command.name().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_BLANK",
                "Name cannot be blank",
                Map.of()
            ));
        }

        // Validation: name length
        if (command.name() != null && command.name().length() > MAX_NAME_LENGTH) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_TOO_LONG",
                "Name must be " + MAX_NAME_LENGTH + " characters or less",
                Map.of("length", command.name().length())
            ));
        }

        // Validation: description length
        if (command.description() != null && command.description().length() > MAX_DESCRIPTION_LENGTH) {
            return Result.failure(new UseCaseError.ValidationError(
                "DESCRIPTION_TOO_LONG",
                "Description must be " + MAX_DESCRIPTION_LENGTH + " characters or less",
                Map.of("length", command.description().length())
            ));
        }

        // Apply changes immutably using toBuilder()
        String newName = command.name() != null ? command.name() : eventType.name();
        String newDescription = command.description() != null ? command.description() : eventType.description();
        EventType updated = eventType.toBuilder()
            .name(newName)
            .description(newDescription)
            .updatedAt(java.time.Instant.now())
            .build();

        // Create domain event
        EventTypeUpdated event = EventTypeUpdated.fromContext(context)
            .eventTypeId(updated.id())
            .name(updated.name())
            .description(updated.description())
            .build();

        return unitOfWork.commit(updated, event, command);
    }
}
