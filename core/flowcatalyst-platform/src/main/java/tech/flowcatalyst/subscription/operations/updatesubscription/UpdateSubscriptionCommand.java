package tech.flowcatalyst.subscription.operations.updatesubscription;

import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.subscription.ConfigEntry;
import tech.flowcatalyst.subscription.EventTypeBinding;
import tech.flowcatalyst.subscription.SubscriptionStatus;

import java.util.List;

/**
 * Command to update an existing subscription.
 *
 * @param subscriptionId The subscription ID to update
 * @param name New display name (null to keep existing)
 * @param description New description (null to keep existing)
 * @param eventTypes New event type bindings (null to keep existing)
 * @param target New target URL (null to keep existing)
 * @param queue New queue name (null to keep existing)
 * @param customConfig New custom config (null to keep existing)
 * @param status New status (null to keep existing)
 * @param maxAgeSeconds New max age (null to keep existing)
 * @param dispatchPoolId New dispatch pool ID (null to keep existing)
 * @param delaySeconds New delay (null to keep existing)
 * @param sequence New sequence (null to keep existing)
 * @param mode New mode (null to keep existing)
 * @param timeoutSeconds New timeout (null to keep existing)
 * @param maxRetries New max retries (null to keep existing)
 * @param serviceAccountId New service account ID (null to keep existing)
 * @param dataOnly New dataOnly flag (null to keep existing)
 */
public record UpdateSubscriptionCommand(
    String subscriptionId,
    String name,
    String description,
    List<EventTypeBinding> eventTypes,
    String target,
    String queue,
    List<ConfigEntry> customConfig,
    SubscriptionStatus status,
    Integer maxAgeSeconds,
    String dispatchPoolId,
    Integer delaySeconds,
    Integer sequence,
    DispatchMode mode,
    Integer timeoutSeconds,
    Integer maxRetries,
    String serviceAccountId,
    Boolean dataOnly
) {}
