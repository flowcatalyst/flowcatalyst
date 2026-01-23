package tech.flowcatalyst.subscription;

/**
 * Source of a subscription - how it was created.
 */
public enum SubscriptionSource {
    /**
     * Defined in code (platform subscriptions).
     * These are synced to the database on startup and cannot be modified via UI/API.
     */
    CODE,

    /**
     * Created or synced via SDK/API.
     */
    API,

    /**
     * Created via the user interface.
     */
    UI
}
