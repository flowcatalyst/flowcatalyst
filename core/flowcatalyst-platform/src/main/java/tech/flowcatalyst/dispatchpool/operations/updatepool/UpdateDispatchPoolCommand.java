package tech.flowcatalyst.dispatchpool.operations.updatepool;

import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;

/**
 * Command to update an existing dispatch pool.
 *
 * @param poolId The pool ID to update
 * @param name New display name (null to keep existing)
 * @param description New description (null to keep existing)
 * @param rateLimit New rate limit (null to keep existing)
 * @param concurrency New concurrency (null to keep existing)
 * @param status New status (null to keep existing)
 */
public record UpdateDispatchPoolCommand(
    String poolId,
    String name,
    String description,
    Integer rateLimit,
    Integer concurrency,
    DispatchPoolStatus status
) {}
