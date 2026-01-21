package tech.flowcatalyst.dispatch;

/**
 * Processing mode for dispatch jobs.
 *
 * Controls how message group ordering and error handling work together.
 * Used by both Subscription (for default behavior) and DispatchJob (for override).
 */
public enum DispatchMode {
    /**
     * No message group ordering guarantees.
     * Jobs are processed as fast as possible, ignoring message groups.
     */
    IMMEDIATE,

    /**
     * Message group ordering with error skip.
     * If a job errors, continue processing the next job in the same message group.
     * Does not block processing of subsequent messages.
     */
    NEXT_ON_ERROR,

    /**
     * Message group ordering with error blocking.
     * If a job errors, stop processing jobs in that message group until resolved.
     * Subsequent messages in the same group will be blocked.
     */
    BLOCK_ON_ERROR
}
