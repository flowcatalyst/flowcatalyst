package tech.flowcatalyst.platform.application.operations.deactivateapplication;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.events.ApplicationDeactivated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for deactivating an Application.
 */
@ApplicationScoped
public class DeactivateApplicationUseCase {

    @Inject
    ApplicationRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<ApplicationDeactivated> execute(
            DeactivateApplicationCommand command,
            ExecutionContext context
    ) {
        // Load aggregate
        Application app = repo.findByIdOptional(command.applicationId())
            .orElse(null);

        if (app == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "APPLICATION_NOT_FOUND",
                "Application not found",
                Map.of("applicationId", command.applicationId())
            ));
        }

        // Check precondition
        if (!app.active) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "ALREADY_DEACTIVATED",
                "Application is already deactivated",
                Map.of("applicationId", command.applicationId())
            ));
        }

        // Apply state change
        app.active = false;

        // Create domain event
        ApplicationDeactivated event = ApplicationDeactivated.fromContext(context)
            .applicationId(app.id)
            .code(app.code)
            .build();

        // Commit atomically
        return unitOfWork.commit(app, event, command);
    }
}
