package tech.flowcatalyst.dispatchpool.operations.deletepool;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolRepository;
import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolDeleted;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.time.Instant;
import java.util.Map;
import java.util.Optional;

/**
 * Use case for deleting (archiving) an existing dispatch pool.
 */
@ApplicationScoped
public class DeleteDispatchPoolUseCase {

    @Inject
    DispatchPoolRepository poolRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<DispatchPoolDeleted> execute(DeleteDispatchPoolCommand command, ExecutionContext context) {
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

        // Cannot delete already archived pools
        if (existing.isArchived()) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "POOL_ALREADY_ARCHIVED",
                "Pool is already archived",
                Map.of("poolId", command.poolId())
            ));
        }

        // Archive the pool (soft delete)
        DispatchPool archived = new DispatchPool(
            existing.id(),
            existing.code(),
            existing.name(),
            existing.description(),
            existing.rateLimit(),
            existing.concurrency(),
            existing.clientId(),
            existing.clientIdentifier(),
            DispatchPoolStatus.ARCHIVED,
            existing.createdAt(),
            Instant.now()
        );

        // Create domain event
        DispatchPoolDeleted event = DispatchPoolDeleted.fromContext(context)
            .poolId(archived.id())
            .code(archived.code())
            .clientId(archived.clientId())
            .clientIdentifier(archived.clientIdentifier())
            .build();

        // Commit atomically
        return unitOfWork.commit(archived, event, command);
    }
}
