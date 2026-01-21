package tech.flowcatalyst.eventtype.operations.createeventtype;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.eventtype.EventTypeStatus;
import tech.flowcatalyst.eventtype.events.EventTypeCreated;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.regex.Pattern;

/**
 * Use case for creating a new EventType.
 *
 * <p>This use case:
 * <ol>
 *   <li>Validates the command (code format, name length, etc.)</li>
 *   <li>Checks that the code is unique</li>
 *   <li>Creates the EventType aggregate</li>
 *   <li>Emits an {@link EventTypeCreated} event</li>
 *   <li>Commits atomically via {@link UnitOfWork}</li>
 * </ol>
 */
@ApplicationScoped
public class CreateEventTypeUseCase {

    /**
     * Code format: {app}:{subdomain}:{aggregate}:{event}
     * Each segment: lowercase alphanumeric with hyphens, starting with letter
     */
    private static final Pattern CODE_PATTERN = Pattern.compile(
        "^[a-z][a-z0-9-]*:[a-z][a-z0-9-]*:[a-z][a-z0-9-]*:[a-z][a-z0-9-]*$"
    );

    private static final int MAX_NAME_LENGTH = 100;
    private static final int MAX_DESCRIPTION_LENGTH = 255;

    @Inject
    EventTypeRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    /**
     * Execute the create event type use case.
     *
     * @param command The command containing event type details
     * @param context The execution context with tracing and principal info
     * @return Success with EventTypeCreated event, or Failure with error
     */
    public Result<EventTypeCreated> execute(
            CreateEventTypeCommand command,
            ExecutionContext context
    ) {
        // Validation: code format
        if (command.code() == null || !CODE_PATTERN.matcher(command.code()).matches()) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_CODE_FORMAT",
                "Code must be in format {app}:{subdomain}:{aggregate}:{event} with lowercase alphanumeric segments",
                Map.of("code", String.valueOf(command.code()))
            ));
        }

        // Authorization check: can principal manage event types with this prefix?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canManageResourceWithPrefix(command.code())) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to create event types for this application",
                Map.of("code", command.code())
            ));
        }

        // Validation: name required
        if (command.name() == null || command.name().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_REQUIRED",
                "Name is required",
                Map.of()
            ));
        }

        // Validation: name length
        if (command.name().length() > MAX_NAME_LENGTH) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_TOO_LONG",
                "Name must be " + MAX_NAME_LENGTH + " characters or less",
                Map.of("length", command.name().length(), "maxLength", MAX_NAME_LENGTH)
            ));
        }

        // Validation: description length
        if (command.description() != null && command.description().length() > MAX_DESCRIPTION_LENGTH) {
            return Result.failure(new UseCaseError.ValidationError(
                "DESCRIPTION_TOO_LONG",
                "Description must be " + MAX_DESCRIPTION_LENGTH + " characters or less",
                Map.of("length", command.description().length(), "maxLength", MAX_DESCRIPTION_LENGTH)
            ));
        }

        // Business rule: code must be unique
        if (repo.existsByCode(command.code())) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CODE_EXISTS",
                "Event type code already exists",
                Map.of("code", command.code())
            ));
        }

        // Create aggregate (immutable record)
        Instant now = Instant.now();
        EventType eventType = new EventType(
            TsidGenerator.generate(),
            command.code().toLowerCase(),
            command.name(),
            command.description(),
            List.of(),  // empty specVersions
            EventTypeStatus.CURRENT,
            now,
            now
        );

        // Create domain event
        EventTypeCreated event = EventTypeCreated.fromContext(context)
            .eventTypeId(eventType.id())
            .code(eventType.code())
            .name(eventType.name())
            .description(eventType.description())
            .build();

        // Atomic commit: entity + event + audit log
        return unitOfWork.commit(eventType, event, command);
    }
}
