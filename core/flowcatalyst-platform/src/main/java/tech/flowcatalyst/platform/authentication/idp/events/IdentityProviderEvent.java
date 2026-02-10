package tech.flowcatalyst.platform.authentication.idp.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all IdentityProvider domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of identity provider events
 * via pattern matching.
 */
public sealed interface IdentityProviderEvent extends DomainEvent
    permits IdentityProviderCreated, IdentityProviderUpdated,
            IdentityProviderDeleted {}
