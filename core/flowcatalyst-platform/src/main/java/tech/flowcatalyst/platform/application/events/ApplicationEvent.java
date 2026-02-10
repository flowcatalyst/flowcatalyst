package tech.flowcatalyst.platform.application.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all Application domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of application events
 * via pattern matching.
 */
public sealed interface ApplicationEvent extends DomainEvent
    permits ApplicationCreated, ApplicationUpdated, ApplicationActivated,
            ApplicationDeactivated, ApplicationDeleted,
            ServiceAccountProvisioned, ApplicationEnabledForClient,
            ApplicationDisabledForClient {}
