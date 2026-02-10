package tech.flowcatalyst.platform.principal.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Sealed hierarchy for all Principal domain events.
 *
 * <p>Enables compiler-enforced exhaustive handling of principal/user events
 * via pattern matching.
 */
public sealed interface PrincipalEvent extends DomainEvent
    permits UserCreated, UserUpdated, UserDeleted, UserActivated,
            UserDeactivated, RolesAssigned, ClientAccessGranted,
            ClientAccessRevoked, ApplicationAccessAssigned {}
