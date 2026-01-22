package tech.flowcatalyst.subscription.operations.syncsubscriptions;

import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.subscription.ConfigEntry;

import java.util.List;

/**
 * Command to bulk sync subscriptions from an external application (SDK).
 *
 * @param applicationCode   The application code (used to scope subscriptions)
 * @param subscriptions     List of subscription definitions to sync
 * @param removeUnlisted    If true, removes API-sourced subscriptions not in the list
 */
public record SyncSubscriptionsCommand(
    String applicationCode,
    List<SyncSubscriptionItem> subscriptions,
    boolean removeUnlisted
) {
    /**
     * Individual subscription item in a sync operation.
     */
    public record SyncSubscriptionItem(
        String code,
        String name,
        String description,
        List<EventTypeBindingItem> eventTypes,
        String target,
        String queue,
        List<ConfigEntry> customConfig,
        Integer maxAgeSeconds,
        String dispatchPoolCode,
        Integer delaySeconds,
        Integer sequence,
        DispatchMode mode,
        Integer timeoutSeconds,
        Integer maxRetries,
        Boolean dataOnly
    ) {}

    /**
     * Event type binding for sync (uses codes instead of IDs).
     */
    public record EventTypeBindingItem(
        String eventTypeCode,
        String specVersion
    ) {}
}
