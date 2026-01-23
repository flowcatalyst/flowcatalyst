package tech.flowcatalyst.dispatchpool.operations.syncpools;

import java.util.List;

/**
 * Command to bulk sync dispatch pools from an external application (SDK).
 *
 * @param applicationCode Application code performing the sync
 * @param pools           List of pool definitions to sync
 * @param removeUnlisted  If true, removes SDK pools not in the list
 */
public record SyncDispatchPoolsCommand(
    String applicationCode,
    List<SyncPoolItem> pools,
    boolean removeUnlisted
) {
    /**
     * Individual pool item in a sync operation.
     *
     * @param code        Unique code for the pool
     * @param name        Display name
     * @param description Optional description
     * @param rateLimit   Maximum dispatches per minute (default 100)
     * @param concurrency Maximum concurrent dispatches (default 10, must be >= 1)
     */
    public record SyncPoolItem(
        String code,
        String name,
        String description,
        Integer rateLimit,
        Integer concurrency
    ) {
        /**
         * Get rate limit with default.
         */
        public int getRateLimitOrDefault() {
            return rateLimit != null ? rateLimit : 100;
        }

        /**
         * Get concurrency with default.
         */
        public int getConcurrencyOrDefault() {
            return concurrency != null ? concurrency : 10;
        }
    }
}
