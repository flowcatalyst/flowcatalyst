package tech.flowcatalyst.eventtype.operations.addschema;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.*;
import tech.flowcatalyst.eventtype.events.SchemaAdded;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;
import java.util.regex.Pattern;

/**
 * Use case for adding a new schema version to an EventType.
 */
@ApplicationScoped
public class AddSchemaUseCase {

    private static final Pattern VERSION_PATTERN = Pattern.compile("^\\d+\\.\\d+$");

    @Inject
    EventTypeRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<SchemaAdded> execute(
            AddSchemaCommand command,
            ExecutionContext context
    ) {
        // Validation: version format
        if (command.version() == null || !VERSION_PATTERN.matcher(command.version()).matches()) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_VERSION_FORMAT",
                "Version must be in MAJOR.MINOR format (e.g., 1.0, 2.1)",
                Map.of("version", String.valueOf(command.version()))
            ));
        }

        // Validation: mimeType required
        if (command.mimeType() == null || command.mimeType().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "MIME_TYPE_REQUIRED",
                "MIME type is required",
                Map.of()
            ));
        }

        // Validation: schema required
        if (command.schema() == null || command.schema().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "SCHEMA_REQUIRED",
                "Schema is required",
                Map.of()
            ));
        }

        // Validation: schemaType required
        if (command.schemaType() == null) {
            return Result.failure(new UseCaseError.ValidationError(
                "SCHEMA_TYPE_REQUIRED",
                "Schema type is required",
                Map.of()
            ));
        }

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

        // Business rule: cannot add schema to archived event type
        if (eventType.status() == EventTypeStatus.ARCHIVE) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "EVENT_TYPE_ARCHIVED",
                "Cannot add schema to archived event type",
                Map.of("eventTypeId", command.eventTypeId(), "status", eventType.status())
            ));
        }

        // Business rule: version must be unique
        if (eventType.hasVersion(command.version())) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "VERSION_EXISTS",
                "Version already exists",
                Map.of("version", command.version())
            ));
        }

        // Create new spec version in FINALISING status
        SpecVersion specVersion = new SpecVersion(
            command.version(),
            command.mimeType(),
            command.schema(),
            command.schemaType(),
            SpecVersionStatus.FINALISING
        );

        // Add spec version immutably
        EventType updated = eventType.addSpecVersion(specVersion);

        // Create domain event
        SchemaAdded event = SchemaAdded.fromContext(context)
            .eventTypeId(updated.id())
            .version(command.version())
            .mimeType(command.mimeType())
            .schema(command.schema())
            .schemaType(command.schemaType())
            .build();

        return unitOfWork.commit(updated, event, command);
    }
}
