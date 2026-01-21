package tech.flowcatalyst.eventtype.operations.finaliseschema;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.*;
import tech.flowcatalyst.eventtype.events.SchemaFinalised;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Use case for finalising a schema version.
 */
@ApplicationScoped
public class FinaliseSchemaUseCase {

    @Inject
    EventTypeRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<SchemaFinalised> execute(
            FinaliseSchemaCommand command,
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

        // Business rule: can only finalise FINALISING schemas
        if (specVersion.status() != SpecVersionStatus.FINALISING) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "INVALID_STATUS",
                "Can only finalise schemas in FINALISING status",
                Map.of("version", command.version(), "currentStatus", specVersion.status())
            ));
        }

        int majorVersion = specVersion.majorVersion();
        String deprecatedVersion = null;

        // Deprecate any existing CURRENT schema with same major version
        List<SpecVersion> updatedVersions = new ArrayList<>();
        for (SpecVersion sv : eventType.specVersions()) {
            if (sv.version().equals(command.version())) {
                // This is the one we're finalising
                updatedVersions.add(sv.withStatus(SpecVersionStatus.CURRENT));
            } else if (sv.majorVersion() == majorVersion && sv.status() == SpecVersionStatus.CURRENT) {
                // Deprecate existing CURRENT with same major version
                updatedVersions.add(sv.withStatus(SpecVersionStatus.DEPRECATED));
                deprecatedVersion = sv.version();
            } else {
                updatedVersions.add(sv);
            }
        }

        // Update immutably
        EventType updated = eventType.withSpecVersions(updatedVersions);

        // Create domain event
        SchemaFinalised event = SchemaFinalised.fromContext(context)
            .eventTypeId(updated.id())
            .version(command.version())
            .deprecatedVersion(deprecatedVersion)
            .build();

        return unitOfWork.commit(updated, event, command);
    }
}
