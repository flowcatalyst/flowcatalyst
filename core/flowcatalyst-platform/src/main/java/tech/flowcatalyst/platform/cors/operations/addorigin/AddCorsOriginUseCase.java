package tech.flowcatalyst.platform.cors.operations.addorigin;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.cors.CorsAllowedOrigin;
import tech.flowcatalyst.platform.cors.CorsAllowedOriginRepository;
import tech.flowcatalyst.platform.cors.events.CorsOriginAdded;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.Map;

/**
 * Use case for adding a new CORS allowed origin.
 */
@ApplicationScoped
public class AddCorsOriginUseCase {

    @Inject
    CorsAllowedOriginRepository repository;

    @Inject
    UnitOfWork unitOfWork;

    public Result<CorsOriginAdded> execute(AddCorsOriginCommand command, ExecutionContext context) {
        // Validate origin is provided
        if (command.origin() == null || command.origin().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "ORIGIN_REQUIRED",
                "Origin is required",
                Map.of()
            ));
        }

        // Normalize origin
        String normalizedOrigin = normalizeOrigin(command.origin());

        // Validate origin format
        if (!isValidOrigin(normalizedOrigin)) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_ORIGIN_FORMAT",
                "Invalid origin format. Must be a valid URL with protocol (e.g., https://app.example.com)",
                Map.of("origin", normalizedOrigin)
            ));
        }

        // Check uniqueness
        if (repository.existsByOrigin(normalizedOrigin)) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "ORIGIN_EXISTS",
                "Origin already exists",
                Map.of("origin", normalizedOrigin)
            ));
        }

        // Create entity
        CorsAllowedOrigin entry = new CorsAllowedOrigin();
        entry.id = TsidGenerator.generate();
        entry.origin = normalizedOrigin;
        entry.description = command.description();
        entry.createdBy = context.principalId();

        // Create domain event
        CorsOriginAdded event = CorsOriginAdded.fromContext(context)
            .originId(entry.id)
            .origin(entry.origin)
            .description(entry.description)
            .build();

        // Commit atomically
        return unitOfWork.commit(entry, event, command);
    }

    private String normalizeOrigin(String origin) {
        if (origin == null) {
            return null;
        }
        origin = origin.toLowerCase().trim();
        if (origin.endsWith("/")) {
            origin = origin.substring(0, origin.length() - 1);
        }
        return origin;
    }

    private boolean isValidOrigin(String origin) {
        if (origin == null || origin.isBlank()) {
            return false;
        }
        // Must start with http:// or https://
        if (!origin.startsWith("http://") && !origin.startsWith("https://")) {
            return false;
        }
        // No wildcards for security
        if (origin.contains("*")) {
            return false;
        }
        // No path component (just protocol + host + optional port)
        String withoutProtocol = origin.replaceFirst("https?://", "");
        if (withoutProtocol.contains("/")) {
            return false;
        }
        return true;
    }
}
