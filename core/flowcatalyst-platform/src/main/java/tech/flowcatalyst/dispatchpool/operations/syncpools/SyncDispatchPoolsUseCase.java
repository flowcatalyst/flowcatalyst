package tech.flowcatalyst.dispatchpool.operations.syncpools;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolRepository;
import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolsSynced;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.time.Instant;
import java.util.*;
import java.util.regex.Pattern;

/**
 * Use case for syncing dispatch pools from an external application (SDK).
 *
 * <p>Syncs anchor-level dispatch pools (clientId = null). Creates new pools,
 * updates existing ones, and optionally removes unlisted pools.
 */
@ApplicationScoped
public class SyncDispatchPoolsUseCase {

    private static final Pattern CODE_PATTERN = Pattern.compile("^[a-z][a-z0-9_-]*$");

    @Inject
    DispatchPoolRepository poolRepo;

    @Inject
    UnitOfWork unitOfWork;

    @Transactional
    public Result<DispatchPoolsSynced> execute(SyncDispatchPoolsCommand command, ExecutionContext context) {
        // Validate command
        if (command.pools() == null || command.pools().isEmpty()) {
            return Result.failure(new UseCaseError.ValidationError(
                "POOLS_REQUIRED",
                "At least one pool is required",
                Map.of()
            ));
        }

        // Validate all pool items
        Result<Void> validationResult = validatePools(command.pools());
        if (validationResult instanceof Result.Failure<Void> f) {
            return Result.failure(f.error());
        }

        Set<String> syncedCodes = new HashSet<>();
        int poolsCreated = 0;
        int poolsUpdated = 0;
        int poolsDeleted = 0;

        for (SyncDispatchPoolsCommand.SyncPoolItem item : command.pools()) {
            String code = item.code().toLowerCase();
            syncedCodes.add(code);

            // Look for existing anchor-level pool with this code
            Optional<DispatchPool> existingOpt = poolRepo.findByCodeAndClientId(code, null);

            if (existingOpt.isPresent()) {
                // Update existing pool
                DispatchPool existing = existingOpt.get();
                DispatchPool updated = existing.toBuilder()
                    .name(item.name() != null ? item.name() : existing.name())
                    .description(item.description())
                    .rateLimit(item.getRateLimitOrDefault())
                    .concurrency(item.getConcurrencyOrDefault())
                    .updatedAt(Instant.now())
                    .build();

                poolRepo.update(updated);
                poolsUpdated++;
            } else {
                // Create new pool
                DispatchPool pool = DispatchPool.create(code, item.name() != null ? item.name() : code)
                    .description(item.description())
                    .rateLimit(item.getRateLimitOrDefault())
                    .concurrency(item.getConcurrencyOrDefault())
                    .clientId(null) // Anchor-level
                    .clientIdentifier(null)
                    .build();

                poolRepo.persist(pool);
                poolsCreated++;
            }
        }

        // Remove unlisted anchor-level pools if requested
        if (command.removeUnlisted()) {
            List<DispatchPool> anchorPools = poolRepo.findAnchorLevel();
            for (DispatchPool pool : anchorPools) {
                if (!syncedCodes.contains(pool.code())) {
                    // Archive instead of hard delete
                    DispatchPool archived = pool.toBuilder()
                        .status(DispatchPoolStatus.ARCHIVED)
                        .updatedAt(Instant.now())
                        .build();
                    poolRepo.update(archived);
                    poolsDeleted++;
                }
            }
        }

        // Create domain event
        DispatchPoolsSynced event = DispatchPoolsSynced.fromContext(context)
            .applicationCode(command.applicationCode())
            .poolsCreated(poolsCreated)
            .poolsUpdated(poolsUpdated)
            .poolsDeleted(poolsDeleted)
            .syncedPoolCodes(new ArrayList<>(syncedCodes))
            .build();

        // Get a reference pool for the commit (use first synced pool)
        // This is needed because UnitOfWork.commit requires an entity
        String firstCode = syncedCodes.iterator().next();
        DispatchPool referencePool = poolRepo.findByCodeAndClientId(firstCode, null).orElseThrow();

        return unitOfWork.commit(referencePool, event, command);
    }

    /**
     * Validate all pool items in the sync request.
     */
    private Result<Void> validatePools(List<SyncDispatchPoolsCommand.SyncPoolItem> pools) {
        Set<String> seenCodes = new HashSet<>();

        for (SyncDispatchPoolsCommand.SyncPoolItem pool : pools) {
            // Validate code is present
            if (pool.code() == null || pool.code().isBlank()) {
                return Result.failure(new UseCaseError.ValidationError(
                    "CODE_REQUIRED",
                    "Pool code is required",
                    Map.of()
                ));
            }

            String code = pool.code().toLowerCase();

            // Validate code format
            if (!CODE_PATTERN.matcher(code).matches()) {
                return Result.failure(new UseCaseError.ValidationError(
                    "INVALID_CODE_FORMAT",
                    "Pool code must be lowercase alphanumeric with hyphens/underscores, starting with a letter",
                    Map.of("code", pool.code())
                ));
            }

            // Check for duplicates in the request
            if (seenCodes.contains(code)) {
                return Result.failure(new UseCaseError.ValidationError(
                    "DUPLICATE_CODE",
                    "Duplicate pool code in sync request",
                    Map.of("code", code)
                ));
            }
            seenCodes.add(code);

            // Validate rate limit if provided
            if (pool.rateLimit() != null && pool.rateLimit() < 1) {
                return Result.failure(new UseCaseError.ValidationError(
                    "INVALID_RATE_LIMIT",
                    "Rate limit must be at least 1",
                    Map.of("code", code, "rateLimit", String.valueOf(pool.rateLimit()))
                ));
            }

            // Validate concurrency if provided
            if (pool.concurrency() != null && pool.concurrency() < 1) {
                return Result.failure(new UseCaseError.ValidationError(
                    "INVALID_CONCURRENCY",
                    "Concurrency must be at least 1",
                    Map.of("code", code, "concurrency", String.valueOf(pool.concurrency()))
                ));
            }
        }

        return Result.success(null);
    }
}
