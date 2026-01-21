package tech.flowcatalyst.platform.application.operations.createapplication;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.events.ApplicationCreated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.Map;
import java.util.regex.Pattern;

/**
 * Use case for creating an Application.
 */
@ApplicationScoped
public class CreateApplicationUseCase {

    private static final Pattern CODE_PATTERN = Pattern.compile("^[a-z][a-z0-9-]*$");

    @Inject
    ApplicationRepository repo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<ApplicationCreated> execute(
            CreateApplicationCommand command,
            ExecutionContext context
    ) {
        // Validate code format
        if (command.code() == null || !CODE_PATTERN.matcher(command.code()).matches()) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_CODE",
                "Invalid application code. Must be lowercase alphanumeric with hyphens, starting with a letter.",
                Map.of("code", command.code() != null ? command.code() : "null")
            ));
        }

        // Check uniqueness
        if (repo.existsByCode(command.code())) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CODE_EXISTS",
                "Application code already exists",
                Map.of("code", command.code())
            ));
        }

        // Validate name
        if (command.name() == null || command.name().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_REQUIRED",
                "Name is required",
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

        // Validate description
        if (command.description() != null && command.description().length() > 1000) {
            return Result.failure(new UseCaseError.ValidationError(
                "DESCRIPTION_TOO_LONG",
                "Description must be 1000 characters or less",
                Map.of("length", command.description().length())
            ));
        }

        // Create the application
        Application app = new Application();
        app.id = TsidGenerator.generate();
        app.type = command.type() != null ? command.type() : Application.ApplicationType.APPLICATION;
        app.code = command.code().toLowerCase();
        app.name = command.name();
        app.description = command.description();
        app.defaultBaseUrl = command.defaultBaseUrl();
        app.iconUrl = command.iconUrl();
        app.website = command.website();
        app.logo = command.logo();
        app.logoMimeType = command.logoMimeType();
        app.active = true;

        // Create domain event
        ApplicationCreated event = ApplicationCreated.fromContext(context)
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
