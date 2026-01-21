/**
 * Unit of Work Pattern
 *
 * Ensures that entity state changes, domain events, and audit logs are
 * committed atomically within a single database transaction.
 *
 * **This is the ONLY way to create a successful Result.** The Result.success()
 * factory requires a token that only UnitOfWork has access to. This guarantees
 * that:
 * - Domain events are always emitted when state changes
 * - Audit logs are always created for operations
 * - Entity state and events are consistent (atomic commit)
 *
 * Usage in a use case:
 * ```typescript
 * async execute(cmd: CreateEventTypeCommand, ctx: ExecutionContext): Promise<Result<EventTypeCreated>> {
 *     // Validation - can return failure directly
 *     if (!isValid(cmd)) {
 *         return Result.failure(UseCaseError.validation('INVALID', 'Invalid input'));
 *     }
 *
 *     // Create aggregate
 *     const eventType = new EventType({ ... });
 *
 *     // Create domain event
 *     const event = new EventTypeCreated(ctx, { eventTypeId: eventType.id, ... });
 *
 *     // Atomic commit - only way to return success
 *     return this.unitOfWork.commit(eventType, event, cmd);
 * }
 * ```
 *
 * Note: This UnitOfWork is for control plane operations only. Event and
 * DispatchJob services are "leaf" operations that do not use this pattern
 * (to avoid circular dependencies).
 */

import type { DomainEvent } from './domain-event.js';
import type { Result } from './result.js';

/**
 * Aggregate entity interface.
 *
 * Entities must have an `id` field that is a TSID string.
 */
export interface Aggregate {
	readonly id: string;
}

/**
 * Unit of Work interface for atomic control plane operations.
 *
 * Implementations must:
 * 1. Execute all operations within a single database transaction
 * 2. Create the domain event record
 * 3. Create the audit log entry
 * 4. Only return Result.success() using the internal token
 */
export interface UnitOfWork {
	/**
	 * Commit an entity change with its domain event atomically.
	 *
	 * Within a single database transaction:
	 * 1. Persists or updates the aggregate entity
	 * 2. Creates the domain event in the events table
	 * 3. Creates the audit log entry
	 *
	 * If any step fails, the entire transaction is rolled back.
	 *
	 * @param aggregate - The entity to persist (must have id field)
	 * @param event - The domain event representing what happened
	 * @param command - The command that was executed (for audit log)
	 * @returns Success with the event, or Failure if transaction fails
	 */
	commit<T extends DomainEvent>(aggregate: Aggregate, event: T, command: unknown): Promise<Result<T>>;

	/**
	 * Commit a delete operation with its domain event atomically.
	 *
	 * Within a single database transaction:
	 * 1. Deletes the aggregate entity
	 * 2. Creates the domain event in the events table
	 * 3. Creates the audit log entry
	 *
	 * @param aggregate - The entity to delete (must have id field)
	 * @param event - The domain event representing the deletion
	 * @param command - The command that was executed (for audit log)
	 * @returns Success with the event, or Failure if transaction fails
	 */
	commitDelete<T extends DomainEvent>(aggregate: Aggregate, event: T, command: unknown): Promise<Result<T>>;

	/**
	 * Commit multiple entity changes with a domain event atomically.
	 *
	 * Use this for operations that create or update multiple aggregates,
	 * such as provisioning a service account (Principal + OAuthClient + Application).
	 *
	 * Within a single database transaction:
	 * 1. Persists or updates all aggregate entities
	 * 2. Creates the domain event in the events table
	 * 3. Creates the audit log entry
	 *
	 * If any step fails, the entire transaction is rolled back.
	 *
	 * @param aggregates - The entities to persist (each must have id field)
	 * @param event - The domain event representing what happened
	 * @param command - The command that was executed (for audit log)
	 * @returns Success with the event, or Failure if transaction fails
	 */
	commitAll<T extends DomainEvent>(aggregates: Aggregate[], event: T, command: unknown): Promise<Result<T>>;
}

/**
 * Abstract base class for UnitOfWork implementations.
 *
 * Provides the success token to implementations for creating successful results.
 */
export { RESULT_SUCCESS_TOKEN, type ResultSuccessToken } from './result.js';
