package tech.flowcatalyst.subscription;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.subscription.events.SubscriptionCreated;
import tech.flowcatalyst.subscription.events.SubscriptionDeleted;
import tech.flowcatalyst.subscription.events.SubscriptionUpdated;
import tech.flowcatalyst.subscription.operations.createsubscription.CreateSubscriptionCommand;
import tech.flowcatalyst.subscription.operations.createsubscription.CreateSubscriptionUseCase;
import tech.flowcatalyst.subscription.operations.deletesubscription.DeleteSubscriptionCommand;
import tech.flowcatalyst.subscription.operations.deletesubscription.DeleteSubscriptionUseCase;
import tech.flowcatalyst.subscription.operations.updatesubscription.UpdateSubscriptionCommand;
import tech.flowcatalyst.subscription.operations.updatesubscription.UpdateSubscriptionUseCase;

import java.util.List;
import java.util.Optional;

/**
 * SubscriptionOperations - Single point of discovery for Subscription aggregate.
 *
 * <p>All write operations on Subscriptions go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all Subscription mutations</li>
 *   <li>Consistent execution context handling</li>
 *   <li>Clear documentation of available operations</li>
 * </ul>
 *
 * <p>Each operation:
 * <ul>
 *   <li>Takes a command describing what to do</li>
 *   <li>Takes an execution context for tracing and principal info</li>
 *   <li>Returns a Result containing either the domain event or an error</li>
 *   <li>Atomically commits the entity, event, and audit log</li>
 * </ul>
 *
 * <p>Read operations do not require execution context and do not emit events.
 */
@ApplicationScoped
public class SubscriptionOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    CreateSubscriptionUseCase createSubscriptionUseCase;

    @Inject
    UpdateSubscriptionUseCase updateSubscriptionUseCase;

    @Inject
    DeleteSubscriptionUseCase deleteSubscriptionUseCase;

    @Inject
    SubscriptionCache subscriptionCache;

    /**
     * Create a new Subscription.
     *
     * @param command The command containing subscription details
     * @param context The execution context
     * @return Success with SubscriptionCreated, or Failure with error
     */
    public Result<SubscriptionCreated> createSubscription(
            CreateSubscriptionCommand command,
            ExecutionContext context
    ) {
        Result<SubscriptionCreated> result = createSubscriptionUseCase.execute(command, context);
        if (result instanceof Result.Success<SubscriptionCreated> success) {
            invalidateCacheForEventTypes(success.value().eventTypes());
        }
        return result;
    }

    /**
     * Update a Subscription's configuration.
     *
     * @param command The command containing update details
     * @param context The execution context
     * @return Success with SubscriptionUpdated, or Failure with error
     */
    public Result<SubscriptionUpdated> updateSubscription(
            UpdateSubscriptionCommand command,
            ExecutionContext context
    ) {
        Result<SubscriptionUpdated> result = updateSubscriptionUseCase.execute(command, context);
        if (result instanceof Result.Success<SubscriptionUpdated> success) {
            invalidateCacheForEventTypes(success.value().eventTypes());
        }
        return result;
    }

    /**
     * Delete a Subscription.
     *
     * @param command The command identifying the subscription to delete
     * @param context The execution context
     * @return Success with SubscriptionDeleted, or Failure with error
     */
    public Result<SubscriptionDeleted> deleteSubscription(
            DeleteSubscriptionCommand command,
            ExecutionContext context
    ) {
        Result<SubscriptionDeleted> result = deleteSubscriptionUseCase.execute(command, context);
        if (result instanceof Result.Success<SubscriptionDeleted> success) {
            invalidateCacheForEventTypes(success.value().eventTypes());
        }
        return result;
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    SubscriptionRepository repo;

    /**
     * Find a Subscription by ID.
     */
    public Optional<Subscription> findById(String id) {
        return repo.findByIdOptional(id);
    }

    /**
     * Find a Subscription by code within a client scope.
     *
     * @param code The subscription code
     * @param clientId The client ID (null for anchor-level subscriptions)
     */
    public Optional<Subscription> findByCodeAndClient(String code, String clientId) {
        return repo.findByCodeAndClient(code, clientId);
    }

    /**
     * Find all Subscriptions.
     */
    public List<Subscription> findAll() {
        return repo.listAll();
    }

    /**
     * Find all active Subscriptions.
     */
    public List<Subscription> findActive() {
        return repo.findActive();
    }

    /**
     * Find Subscriptions for a specific client.
     */
    public List<Subscription> findByClientId(String clientId) {
        return repo.findByClientId(clientId);
    }

    /**
     * Find anchor-level Subscriptions (clientId is null).
     */
    public List<Subscription> findAnchorLevel() {
        return repo.findAnchorLevel();
    }

    /**
     * Find Subscriptions using a specific dispatch pool.
     */
    public List<Subscription> findByDispatchPoolId(String dispatchPoolId) {
        return repo.findByDispatchPoolId(dispatchPoolId);
    }

    /**
     * Check if any subscriptions are using a specific dispatch pool.
     */
    public boolean hasSubscriptionsForPool(String dispatchPoolId) {
        return repo.existsByDispatchPoolId(dispatchPoolId);
    }

    /**
     * Find Subscriptions that listen to a specific event type.
     */
    public List<Subscription> findByEventTypeId(String eventTypeId) {
        return repo.findByEventTypeId(eventTypeId);
    }

    /**
     * Find active subscriptions for a specific event type and client.
     * Used for matching events to subscriptions.
     */
    public List<Subscription> findActiveByEventTypeAndClient(String eventTypeId, String clientId) {
        return repo.findActiveByEventTypeAndClient(eventTypeId, clientId);
    }

    /**
     * Find Subscriptions with filters.
     *
     * @param clientId Filter by client (null to skip)
     * @param status Filter by status
     * @param source Filter by source
     * @param dispatchPoolId Filter by dispatch pool
     */
    public List<Subscription> findWithFilters(String clientId, SubscriptionStatus status,
                                               SubscriptionSource source, String dispatchPoolId) {
        return repo.findWithFilters(clientId, status, source, dispatchPoolId);
    }

    // ========================================================================
    // Cache Invalidation
    // ========================================================================

    /**
     * Invalidate subscription cache for the given event type bindings.
     */
    private void invalidateCacheForEventTypes(List<EventTypeBinding> eventTypes) {
        if (eventTypes == null) {
            return;
        }
        for (EventTypeBinding binding : eventTypes) {
            subscriptionCache.invalidateByEventTypeCode(binding.eventTypeCode());
        }
    }
}
