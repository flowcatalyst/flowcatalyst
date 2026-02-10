package tech.flowcatalyst.platform.authentication.domain.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all EmailDomainMapping domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of email domain mapping events
 * via pattern matching.
 */
public sealed interface EmailDomainMappingEvent extends DomainEvent
    permits EmailDomainMappingCreated, EmailDomainMappingUpdated,
            EmailDomainMappingDeleted {}
