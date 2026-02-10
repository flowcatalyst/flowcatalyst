package tech.flowcatalyst.subscription.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all Subscription domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of subscription events
 * via pattern matching.
 */
public sealed interface SubscriptionEvent extends DomainEvent
    permits SubscriptionCreated, SubscriptionUpdated, SubscriptionDeleted,
            SubscriptionsSynced {}
