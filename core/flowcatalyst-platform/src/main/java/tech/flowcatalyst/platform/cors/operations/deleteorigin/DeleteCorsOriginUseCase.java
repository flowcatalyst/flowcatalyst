package tech.flowcatalyst.platform.cors.operations.deleteorigin;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClientRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.cors.CorsAllowedOrigin;
import tech.flowcatalyst.platform.cors.CorsAllowedOriginRepository;
import tech.flowcatalyst.platform.cors.events.CorsOriginDeleted;

import java.util.List;
import java.util.Map;

/**
 * Use case for deleting a CORS allowed origin.
 */
@ApplicationScoped
public class DeleteCorsOriginUseCase {

    @Inject
    CorsAllowedOriginRepository repository;

    @Inject
    OAuthClientRepository oauthClientRepository;

    @Inject
    UnitOfWork unitOfWork;

    public Result<CorsOriginDeleted> execute(DeleteCorsOriginCommand command, ExecutionContext context) {
        // Validate ID is provided
        if (command.originId() == null || command.originId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "ORIGIN_ID_REQUIRED",
                "Origin ID is required",
                Map.of()
            ));
        }

        // Find the entry
        CorsAllowedOrigin entry = repository.findById(command.originId()).orElse(null);
        if (entry == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "ORIGIN_NOT_FOUND",
                "CORS origin not found",
                Map.of("originId", command.originId())
            ));
        }

        // Check if this origin is used by any OAuth client
        List<String> usingClients = oauthClientRepository.findClientNamesUsingOrigin(entry.origin);
        if (!usingClients.isEmpty()) {
            String clientNames = String.join(", ", usingClients);
            return Result.failure(new UseCaseError.ValidationError(
                "ORIGIN_IN_USE",
                "Cannot delete origin that is used by OAuth clients: " + clientNames,
                Map.of("origin", entry.origin, "clients", usingClients)
            ));
        }

        // Create domain event
        CorsOriginDeleted event = CorsOriginDeleted.fromContext(context)
            .originId(entry.id)
            .origin(entry.origin)
            .build();

        // Commit delete atomically
        return unitOfWork.commitDelete(entry, event, command);
    }
}
