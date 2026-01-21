package tech.flowcatalyst.subscription;

/**
 * Status of a subscription.
 */
public enum SubscriptionStatus {
    /**
     * Subscription is active and will create dispatch jobs for matching events.
     */
    ACTIVE,

    /**
     * Subscription is paused and will not create dispatch jobs.
     */
    PAUSED
}
