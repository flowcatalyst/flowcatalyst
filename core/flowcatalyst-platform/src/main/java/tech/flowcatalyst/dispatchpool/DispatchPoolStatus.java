package tech.flowcatalyst.dispatchpool;

/**
 * Status of a dispatch pool.
 */
public enum DispatchPoolStatus {
    /**
     * Pool is active and can process dispatch jobs.
     */
    ACTIVE,

    /**
     * Pool is temporarily suspended and will not process jobs.
     */
    SUSPENDED,

    /**
     * Pool is archived (soft-deleted) and cannot be used.
     */
    ARCHIVED
}
