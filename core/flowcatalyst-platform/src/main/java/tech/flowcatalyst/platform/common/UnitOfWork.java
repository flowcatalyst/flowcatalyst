package tech.flowcatalyst.platform.common;

/**
 * Unit of Work for atomic control plane operations.
 *
 * <p>Ensures that entity state changes, domain events, and audit logs are
 * committed atomically within a single MongoDB transaction.
 *
 * <p><strong>This is the ONLY way to create a successful {@link Result}.</strong>
 * The {@link Result#success(Object)} method is package-private, so use cases
 * must go through UnitOfWork to return success. This guarantees that:
 * <ul>
 *   <li>Domain events are always emitted when state changes</li>
 *   <li>Audit logs are always created for operations</li>
 *   <li>Entity state and events are consistent (atomic commit)</li>
 * </ul>
 *
 * <p>Usage in a use case:
 * <pre>{@code
 * public Result<EventTypeCreated> execute(CreateEventTypeCommand cmd, ExecutionContext ctx) {
 *     // Validation - can return failure directly
 *     if (!isValid(cmd)) {
 *         return Result.failure(new ValidationError(...));
 *     }
 *
 *     // Create aggregate
 *     EventType eventType = new EventType(...);
 *
 *     // Create domain event
 *     EventTypeCreated event = EventTypeCreated.builder()
 *         .from(ctx)
 *         .eventTypeId(eventType.id)
 *         .build();
 *
 *     // Atomic commit - only way to return success
 *     return unitOfWork.commit(eventType, event, cmd);
 * }
 * }</pre>
 *
 * <p><strong>Note:</strong> This UnitOfWork is for control plane operations only.
 * {@link tech.flowcatalyst.event.EventService} and
 * {@link tech.flowcatalyst.dispatchjob.service.DispatchJobService} are leaf
 * operations that do not use this pattern (to avoid circular dependencies).
 */
public interface UnitOfWork {

    /**
     * Commit an entity change with its domain event atomically.
     *
     * <p>Within a single MongoDB transaction:
     * <ol>
     *   <li>Persists or updates the aggregate entity</li>
     *   <li>Creates the domain event in the events collection</li>
     *   <li>Creates the audit log entry</li>
     * </ol>
     *
     * <p>If any step fails, the entire transaction is rolled back.
     *
     * @param aggregate The entity to persist (must have public Long id field)
     * @param event     The domain event representing what happened
     * @param command   The command that was executed (for audit log)
     * @param <T>       The domain event type
     * @return Success with the event, or Failure if transaction fails
     */
    <T extends DomainEvent> Result<T> commit(
        Object aggregate,
        T event,
        Object command
    );

    /**
     * Commit a delete operation with its domain event atomically.
     *
     * <p>Within a single MongoDB transaction:
     * <ol>
     *   <li>Deletes the aggregate entity</li>
     *   <li>Creates the domain event in the events collection</li>
     *   <li>Creates the audit log entry</li>
     * </ol>
     *
     * @param aggregate The entity to delete (must have public Long id field)
     * @param event     The domain event representing the deletion
     * @param command   The command that was executed (for audit log)
     * @param <T>       The domain event type
     * @return Success with the event, or Failure if transaction fails
     */
    <T extends DomainEvent> Result<T> commitDelete(
        Object aggregate,
        T event,
        Object command
    );

    /**
     * Commit multiple entity changes with a domain event atomically.
     *
     * <p>Use this for operations that create or update multiple aggregates,
     * such as provisioning a service account (Principal + OAuthClient + Application).
     *
     * <p>Within a single MongoDB transaction:
     * <ol>
     *   <li>Persists or updates all aggregate entities</li>
     *   <li>Creates the domain event in the events collection</li>
     *   <li>Creates the audit log entry</li>
     * </ol>
     *
     * <p>If any step fails, the entire transaction is rolled back.
     *
     * @param aggregates The entities to persist (each must have public String id field)
     * @param event      The domain event representing what happened
     * @param command    The command that was executed (for audit log)
     * @param <T>        The domain event type
     * @return Success with the event, or Failure if transaction fails
     */
    <T extends DomainEvent> Result<T> commitAll(
        java.util.List<Object> aggregates,
        T event,
        Object command
    );
}
