package tech.flowcatalyst.eventtype.operations.createeventtype;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.eventtype.EventTypeSource;
import tech.flowcatalyst.eventtype.EventTypeStatus;
import tech.flowcatalyst.eventtype.events.EventTypeCreated;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.EntityType;
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
 *   <li>Validates the command (code segments, name length, etc.)</li>
 *   <li>Checks that the code is unique</li>
 *   <li>Creates the EventType aggregate</li>
 *   <li>Emits an {@link EventTypeCreated} event</li>
 *   <li>Commits atomically via {@link UnitOfWork}</li>
 * </ol>
 */
@ApplicationScoped
public class CreateEventTypeUseCase {

    /**
     * Segment format: lowercase alphanumeric with hyphens, starting with letter
     */
    private static final Pattern SEGMENT_PATTERN = Pattern.compile(
        "^[a-z][a-z0-9-]*$"
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
        // Validation: each segment must be valid
        Result<Void> segmentValidation = validateSegments(command);
        if (segmentValidation instanceof Result.Failure<Void> f) {
            return Result.failure(f.error());
        }

        // Build the full code from segments
        String code = command.buildCode();

        // Authorization check: can principal manage event types with this prefix?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canManageResourceWithPrefix(code)) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to create event types for this application",
                Map.of("application", command.application())
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
        if (repo.existsByCode(code)) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CODE_EXISTS",
                "Event type code already exists",
                Map.of("code", code)
            ));
        }

        // Create aggregate (immutable record)
        Instant now = Instant.now();
        EventType eventType = new EventType(
            TsidGenerator.generate(EntityType.EVENT_TYPE),
            code,
            command.name(),
            command.description(),
            List.of(),  // empty specVersions
            EventTypeStatus.CURRENT,
            EventTypeSource.UI,  // Created via UI/API, not sync
            command.clientScoped(),
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

    /**
     * Validate all code segments.
     */
    private Result<Void> validateSegments(CreateEventTypeCommand command) {
        // Validate application segment
        Result<Void> appResult = validateSegment("application", command.application());
        if (appResult instanceof Result.Failure<Void>) {
            return appResult;
        }

        // Validate subdomain segment
        Result<Void> subResult = validateSegment("subdomain", command.subdomain());
        if (subResult instanceof Result.Failure<Void>) {
            return subResult;
        }

        // Validate aggregate segment
        Result<Void> aggResult = validateSegment("aggregate", command.aggregate());
        if (aggResult instanceof Result.Failure<Void>) {
            return aggResult;
        }

        // Validate event segment
        Result<Void> eventResult = validateSegment("event", command.event());
        if (eventResult instanceof Result.Failure<Void>) {
            return eventResult;
        }

        return Result.success(null);
    }

    /**
     * Validate a single code segment.
     */
    private Result<Void> validateSegment(String fieldName, String value) {
        if (value == null || value.isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                fieldName.toUpperCase() + "_REQUIRED",
                fieldName.substring(0, 1).toUpperCase() + fieldName.substring(1) + " is required",
                Map.of("field", fieldName)
            ));
        }

        if (!SEGMENT_PATTERN.matcher(value).matches()) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_" + fieldName.toUpperCase() + "_FORMAT",
                fieldName.substring(0, 1).toUpperCase() + fieldName.substring(1) +
                    " must be lowercase alphanumeric with hyphens, starting with a letter",
                Map.of("field", fieldName, "value", value)
            ));
        }

        return Result.success(null);
    }
}
