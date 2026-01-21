package tech.flowcatalyst.subscription;

/**
 * Source of a subscription - how it was created.
 */
public enum SubscriptionSource {
    /**
     * Created or synced via SDK/API.
     */
    API,

    /**
     * Created via the user interface.
     */
    UI
}
