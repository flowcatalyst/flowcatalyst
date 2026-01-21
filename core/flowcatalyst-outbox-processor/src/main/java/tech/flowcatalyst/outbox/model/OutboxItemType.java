package tech.flowcatalyst.outbox.model;

/**
 * Type of item in the outbox.
 */
public enum OutboxItemType {
    /**
     * An event to be sent to the FlowCatalyst Events API.
     */
    EVENT,

    /**
     * A dispatch job to be sent to the FlowCatalyst Dispatch Jobs API.
     */
    DISPATCH_JOB
}
