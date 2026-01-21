package tech.flowcatalyst.dispatchpool.operations.createpool;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolRepository;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolCreated;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;
import java.util.Optional;

/**
 * Use case for creating a new dispatch pool.
 */
@ApplicationScoped
public class CreateDispatchPoolUseCase {

    @Inject
    DispatchPoolRepository poolRepo;

    @Inject
    ClientRepository clientRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<DispatchPoolCreated> execute(CreateDispatchPoolCommand command, ExecutionContext context) {
        // Validate code
        if (command.code() == null || command.code().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "CODE_REQUIRED",
                "Code is required",
                Map.of()
            ));
        }

        // Validate code format (lowercase alphanumeric with hyphens)
        if (!isValidCode(command.code())) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_CODE_FORMAT",
                "Code must be lowercase alphanumeric with hyphens, starting with a letter",
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

        // Validate client (if provided)
        String clientIdentifier = null;
        if (command.clientId() != null && !command.clientId().isBlank()) {
            Optional<Client> clientOpt = clientRepo.findByIdOptional(command.clientId());
            if (clientOpt.isEmpty()) {
                return Result.failure(new UseCaseError.NotFoundError(
                    "CLIENT_NOT_FOUND",
                    "Client not found",
                    Map.of("clientId", command.clientId())
                ));
            }
            clientIdentifier = clientOpt.get().identifier;
        }

        // Validate rate limit
        if (command.rateLimit() < 1) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_RATE_LIMIT",
                "Rate limit must be at least 1",
                Map.of("rateLimit", String.valueOf(command.rateLimit()))
            ));
        }

        // Validate concurrency
        if (command.concurrency() < 1) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_CONCURRENCY",
                "Concurrency must be at least 1",
                Map.of("concurrency", String.valueOf(command.concurrency()))
            ));
        }

        // Check code uniqueness within scope (code + clientId)
        if (poolRepo.existsByCodeAndClientId(command.code(), command.clientId())) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CODE_EXISTS",
                "A pool with this code already exists in this scope",
                Map.of("code", command.code())
            ));
        }

        // Create pool using safe builder pattern
        DispatchPool pool = DispatchPool.create(command.code(), command.name())
            .description(command.description())
            .rateLimit(command.rateLimit())
            .concurrency(command.concurrency())
            .clientId(command.clientId())
            .clientIdentifier(clientIdentifier)
            .build();

        // Create domain event
        DispatchPoolCreated event = DispatchPoolCreated.fromContext(context)
            .poolId(pool.id())
            .code(pool.code())
            .name(pool.name())
            .description(pool.description())
            .rateLimit(pool.rateLimit())
            .concurrency(pool.concurrency())
            .clientId(pool.clientId())
            .clientIdentifier(pool.clientIdentifier())
            .status(pool.status())
            .build();

        // Commit atomically
        return unitOfWork.commit(pool, event, command);
    }

    private boolean isValidCode(String code) {
        if (code == null || code.isBlank()) {
            return false;
        }
        // Lowercase alphanumeric with hyphens, must start with letter
        return code.matches("^[a-z][a-z0-9-]*$");
    }
}
