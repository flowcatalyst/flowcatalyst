package tech.flowcatalyst.serviceaccount.events;

import tech.flowcatalyst.platform.common.DomainEvent;

/**
 * Marker interface for all ServiceAccount domain events.
 *
 * <p>Groups service account events under a common type for pattern matching
 * and event routing.
 *
 * <p>Note: This is a non-sealed interface rather than a sealed one because
 * service account events are co-located with their use cases across multiple
 * operation packages. Java sealed types require permitted subtypes to be in
 * the same package (for unnamed modules). If these events are later
 * consolidated into a single events package, this can become sealed.
 *
 * <p>Known implementations:
 * <ul>
 *   <li>{@code ServiceAccountCreated}</li>
 *   <li>{@code ServiceAccountDeleted}</li>
 *   <li>{@code ServiceAccountUpdated}</li>
 *   <li>{@code RolesAssigned}</li>
 *   <li>{@code AuthTokenRegenerated}</li>
 *   <li>{@code SigningSecretRegenerated}</li>
 * </ul>
 */
public interface ServiceAccountEvent extends DomainEvent {}
