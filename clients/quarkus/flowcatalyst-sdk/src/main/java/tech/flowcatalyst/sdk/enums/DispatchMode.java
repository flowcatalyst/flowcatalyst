package tech.flowcatalyst.sdk.enums;

/**
 * Dispatch mode for subscriptions.
 */
public enum DispatchMode {
    /** No ordering guarantees, maximum throughput */
    IMMEDIATE,

    /** Block message group on error until resolved */
    BLOCK_ON_ERROR
}
