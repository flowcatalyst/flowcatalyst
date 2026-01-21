package tech.flowcatalyst.platform.application.operations.updateapplication;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.events.ApplicationUpdated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for updating an Application.
 */
@ApplicationScoped
public class UpdateApplicationUseCase {

    @Inject
    ApplicationRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<ApplicationUpdated> execute(
            UpdateApplicationCommand command,
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

        // Apply updates
        if (command.name() != null) {
            if (command.name().isBlank()) {
                return Result.failure(new UseCaseError.ValidationError(
                    "NAME_BLANK",
                    "Name cannot be blank",
                    Map.of()
                ));
            }
            if (command.name().length() > 255) {
                return Result.failure(new UseCaseError.ValidationError(
                    "NAME_TOO_LONG",
                    "Name must be 255 characters or less",
                    Map.of("length", command.name().length())
                ));
            }
            app.name = command.name();
        }

        if (command.description() != null) {
            if (command.description().length() > 1000) {
                return Result.failure(new UseCaseError.ValidationError(
                    "DESCRIPTION_TOO_LONG",
                    "Description must be 1000 characters or less",
                    Map.of("length", command.description().length())
                ));
            }
            app.description = command.description();
        }

        if (command.defaultBaseUrl() != null) {
            app.defaultBaseUrl = command.defaultBaseUrl();
        }

        if (command.iconUrl() != null) {
            app.iconUrl = command.iconUrl();
        }

        if (command.website() != null) {
            app.website = command.website().isBlank() ? null : command.website();
        }

        if (command.logo() != null) {
            app.logo = command.logo().isBlank() ? null : command.logo();
        }

        if (command.logoMimeType() != null) {
            app.logoMimeType = command.logoMimeType().isBlank() ? null : command.logoMimeType();
        }

        // Update timestamp
        app.updatedAt = java.time.Instant.now();

        // Create domain event
        ApplicationUpdated event = ApplicationUpdated.fromContext(context)
            .applicationId(app.id)
            .code(app.code)
            .name(app.name)
            .description(app.description)
            .defaultBaseUrl(app.defaultBaseUrl)
            .iconUrl(app.iconUrl)
            .website(app.website)
            .logoMimeType(app.logoMimeType)
            .build();

        // Commit atomically
        return unitOfWork.commit(app, event, command);
    }
}
