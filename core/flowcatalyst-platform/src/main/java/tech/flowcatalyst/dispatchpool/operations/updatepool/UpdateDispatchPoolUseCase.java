package tech.flowcatalyst.dispatchpool.operations.updatepool;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolRepository;
import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolUpdated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.time.Instant;
import java.util.Map;
import java.util.Optional;

/**
 * Use case for updating an existing dispatch pool.
 */
@ApplicationScoped
public class UpdateDispatchPoolUseCase {

    @Inject
    DispatchPoolRepository poolRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<DispatchPoolUpdated> execute(UpdateDispatchPoolCommand command, ExecutionContext context) {
        // Validate pool ID
        if (command.poolId() == null || command.poolId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "POOL_ID_REQUIRED",
                "Pool ID is required",
                Map.of()
            ));
        }

        // Find existing pool
        Optional<DispatchPool> existingOpt = poolRepo.findByIdOptional(command.poolId());
        if (existingOpt.isEmpty()) {
            return Result.failure(new UseCaseError.NotFoundError(
                "POOL_NOT_FOUND",
                "Dispatch pool not found",
                Map.of("poolId", command.poolId())
            ));
        }

        DispatchPool existing = existingOpt.get();

        // Cannot update archived pools
        if (existing.isArchived()) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "POOL_ARCHIVED",
                "Cannot update an archived pool",
                Map.of("poolId", command.poolId())
            ));
        }

        // Validate rate limit if provided
        if (command.rateLimit() != null && command.rateLimit() < 1) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_RATE_LIMIT",
                "Rate limit must be at least 1",
                Map.of("rateLimit", String.valueOf(command.rateLimit()))
            ));
        }

        // Validate concurrency if provided
        if (command.concurrency() != null && command.concurrency() < 1) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_CONCURRENCY",
                "Concurrency must be at least 1",
                Map.of("concurrency", String.valueOf(command.concurrency()))
            ));
        }

        // Apply updates
        String newName = command.name() != null ? command.name() : existing.name();
        String newDescription = command.description() != null ? command.description() : existing.description();
        int newRateLimit = command.rateLimit() != null ? command.rateLimit() : existing.rateLimit();
        int newConcurrency = command.concurrency() != null ? command.concurrency() : existing.concurrency();
        DispatchPoolStatus newStatus = command.status() != null ? command.status() : existing.status();

        // Create updated pool
        DispatchPool updated = new DispatchPool(
            existing.id(),
            existing.code(),
            newName,
            newDescription,
            newRateLimit,
            newConcurrency,
            existing.clientId(),
            existing.clientIdentifier(),
            newStatus,
            existing.createdAt(),
            Instant.now()
        );

        // Create domain event
        DispatchPoolUpdated event = DispatchPoolUpdated.fromContext(context)
            .poolId(updated.id())
            .code(updated.code())
            .name(updated.name())
            .description(updated.description())
            .rateLimit(updated.rateLimit())
            .concurrency(updated.concurrency())
            .clientId(updated.clientId())
            .clientIdentifier(updated.clientIdentifier())
            .status(updated.status())
            .build();

        // Commit atomically
        return unitOfWork.commit(updated, event, command);
    }
}
