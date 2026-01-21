package tech.flowcatalyst.dispatchpool.operations.createpool;

/**
 * Command to create a new dispatch pool.
 *
 * @param code Unique code within client scope
 * @param name Display name
 * @param description Optional description
 * @param rateLimit Maximum dispatches per minute
 * @param concurrency Maximum concurrent dispatches (must be >= 1)
 * @param clientId Client ID (nullable - null for anchor-level pools)
 */
public record CreateDispatchPoolCommand(
    String code,
    String name,
    String description,
    int rateLimit,
    int concurrency,
    String clientId
) {}
