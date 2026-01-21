package tech.flowcatalyst.eventtype.operations.deprecateschema;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.*;
import tech.flowcatalyst.eventtype.events.SchemaDeprecated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Use case for deprecating a schema version.
 */
@ApplicationScoped
public class DeprecateSchemaUseCase {

    @Inject
    EventTypeRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<SchemaDeprecated> execute(
            DeprecateSchemaCommand command,
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

        // Find the spec version
        SpecVersion specVersion = eventType.findSpecVersion(command.version());
        if (specVersion == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "VERSION_NOT_FOUND",
                "Version not found",
                Map.of("version", command.version())
            ));
        }

        // Business rule: cannot deprecate FINALISING schemas
        if (specVersion.status() == SpecVersionStatus.FINALISING) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CANNOT_DEPRECATE_FINALISING",
                "Cannot deprecate FINALISING schemas. Finalise or delete them first.",
                Map.of("version", command.version(), "currentStatus", specVersion.status())
            ));
        }

        // Business rule: already deprecated
        if (specVersion.status() == SpecVersionStatus.DEPRECATED) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "ALREADY_DEPRECATED",
                "Schema is already deprecated",
                Map.of("version", command.version())
            ));
        }

        // Update the version status immutably
        List<SpecVersion> updatedVersions = new ArrayList<>();
        for (SpecVersion sv : eventType.specVersions()) {
            if (sv.version().equals(command.version())) {
                updatedVersions.add(sv.withStatus(SpecVersionStatus.DEPRECATED));
            } else {
                updatedVersions.add(sv);
            }
        }

        EventType updated = eventType.withSpecVersions(updatedVersions);

        // Create domain event
        SchemaDeprecated event = SchemaDeprecated.fromContext(context)
            .eventTypeId(updated.id())
            .version(command.version())
            .build();

        return unitOfWork.commit(updated, event, command);
    }
}
