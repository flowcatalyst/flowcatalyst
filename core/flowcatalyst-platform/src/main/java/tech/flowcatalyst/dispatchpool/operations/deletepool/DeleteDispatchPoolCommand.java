package tech.flowcatalyst.dispatchpool.operations.deletepool;

/**
 * Command to delete (archive) an existing dispatch pool.
 *
 * @param poolId The pool ID to delete
 */
public record DeleteDispatchPoolCommand(
    String poolId
) {}
