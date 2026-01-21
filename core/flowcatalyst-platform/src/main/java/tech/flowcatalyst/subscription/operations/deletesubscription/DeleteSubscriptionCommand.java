package tech.flowcatalyst.subscription.operations.deletesubscription;

/**
 * Command to delete an existing subscription.
 *
 * @param subscriptionId The subscription ID to delete
 */
public record DeleteSubscriptionCommand(
    String subscriptionId
) {}
