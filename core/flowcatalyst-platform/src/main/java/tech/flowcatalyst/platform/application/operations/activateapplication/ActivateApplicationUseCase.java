package tech.flowcatalyst.platform.application.operations.activateapplication;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.events.ApplicationActivated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for activating an Application.
 */
@ApplicationScoped
public class ActivateApplicationUseCase {

    @Inject
    ApplicationRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<ApplicationActivated> execute(
            ActivateApplicationCommand command,
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
        if (app.active) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "ALREADY_ACTIVE",
                "Application is already active",
                Map.of("applicationId", command.applicationId())
            ));
        }

        // Apply state change
        app.active = true;

        // Create domain event
        ApplicationActivated event = ApplicationActivated.fromContext(context)
            .applicationId(app.id)
            .code(app.code)
            .build();

        // Commit atomically
        return unitOfWork.commit(app, event, command);
    }
}
