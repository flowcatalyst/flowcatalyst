/**
 * Drizzle Transactional Unit of Work
 *
 * Concrete implementation of UnitOfWork that uses DrizzleORM transactions
 * to atomically commit entity changes, domain events, and audit logs.
 *
 * This is the ONLY way to create successful Results, guaranteeing that
 * domain events and audit logs are always created alongside state changes.
 */

import {
	type UnitOfWork,
	type Aggregate,
	type DomainEvent,
	Result,
	RESULT_SUCCESS_TOKEN,
	UseCaseError,
	DomainEvent as DomainEventUtils,
} from '@flowcatalyst/domain-core';
import { generate } from '@flowcatalyst/tsid';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { AggregateRegistry } from './aggregate-registry.js';
import type { TransactionContext, TransactionManager } from './transaction.js';
import { events, type NewEvent, type EventContextData } from './schema/events.js';
import { auditLogs, type NewAuditLog } from './schema/audit-logs.js';

/**
 * Configuration for the Drizzle Unit of Work.
 */
export interface DrizzleUnitOfWorkConfig {
	/** Transaction manager for database operations */
	readonly transactionManager: TransactionManager;
	/** Registry for dispatching aggregate persistence */
	readonly aggregateRegistry: AggregateRegistry;
	/** Optional: Function to extract client ID from aggregates */
	readonly extractClientId?: (aggregate: Aggregate) => string | null;
}

/**
 * Create a Drizzle-based Unit of Work.
 *
 * @param config - Configuration options
 * @returns UnitOfWork implementation
 *
 * @example
 * ```typescript
 * const unitOfWork = createDrizzleUnitOfWork({
 *     transactionManager: createTransactionManager(db),
 *     aggregateRegistry: registry,
 * });
 *
 * // In a use case:
 * const event = new UserCreatedEvent(ctx, userData);
 * return unitOfWork.commit(user, event, createUserCommand);
 * ```
 */
export function createDrizzleUnitOfWork(config: DrizzleUnitOfWorkConfig): UnitOfWork {
	const { transactionManager, aggregateRegistry, extractClientId } = config;

	return {
		async commit<T extends DomainEvent>(aggregate: Aggregate, event: T, command: unknown): Promise<Result<T>> {
			try {
				return await transactionManager.inTransaction(async (tx) => {
					// 1. Persist the aggregate
					await aggregateRegistry.persist(aggregate as never, tx);

					// 2. Create the event record
					await createEventRecord(tx.db, event, extractClientId?.(aggregate) ?? null);

					// 3. Create the audit log
					await createAuditLogRecord(tx.db, event, command);

					// 4. Return success (only UnitOfWork can do this)
					return Result.success(RESULT_SUCCESS_TOKEN, event);
				});
			} catch (error) {
				return Result.failure(
					UseCaseError.businessRule(
						'COMMIT_FAILED',
						error instanceof Error ? error.message : 'Unknown error during commit',
						{ cause: error instanceof Error ? error.name : 'Unknown' },
					),
				);
			}
		},

		async commitDelete<T extends DomainEvent>(aggregate: Aggregate, event: T, command: unknown): Promise<Result<T>> {
			try {
				return await transactionManager.inTransaction(async (tx) => {
					// 1. Delete the aggregate
					await aggregateRegistry.delete(aggregate as never, tx);

					// 2. Create the event record
					await createEventRecord(tx.db, event, extractClientId?.(aggregate) ?? null);

					// 3. Create the audit log
					await createAuditLogRecord(tx.db, event, command);

					// 4. Return success
					return Result.success(RESULT_SUCCESS_TOKEN, event);
				});
			} catch (error) {
				return Result.failure(
					UseCaseError.businessRule(
						'DELETE_FAILED',
						error instanceof Error ? error.message : 'Unknown error during delete',
						{ cause: error instanceof Error ? error.name : 'Unknown' },
					),
				);
			}
		},

		async commitAll<T extends DomainEvent>(
			aggregates: Aggregate[],
			event: T,
			command: unknown,
		): Promise<Result<T>> {
			try {
				return await transactionManager.inTransaction(async (tx) => {
					// 1. Persist all aggregates
					for (const aggregate of aggregates) {
						await aggregateRegistry.persist(aggregate as never, tx);
					}

					// 2. Create the event record (use first aggregate for client ID)
					const firstAggregate = aggregates[0];
					const clientId = firstAggregate !== undefined ? (extractClientId?.(firstAggregate) ?? null) : null;
					await createEventRecord(tx.db, event, clientId);

					// 3. Create the audit log
					await createAuditLogRecord(tx.db, event, command);

					// 4. Return success
					return Result.success(RESULT_SUCCESS_TOKEN, event);
				});
			} catch (error) {
				return Result.failure(
					UseCaseError.businessRule(
						'COMMIT_ALL_FAILED',
						error instanceof Error ? error.message : 'Unknown error during commit',
						{ cause: error instanceof Error ? error.name : 'Unknown' },
					),
				);
			}
		},
	};
}

/**
 * Create an event record in the database.
 */
async function createEventRecord(db: PostgresJsDatabase, event: DomainEvent, clientId: string | null): Promise<void> {
	const contextData: EventContextData[] = [];

	// Add aggregate type and ID to context data for filtering
	const aggregateType = DomainEventUtils.extractAggregateType(event.subject);
	const entityId = DomainEventUtils.extractEntityId(event.subject);

	if (aggregateType !== 'Unknown') {
		contextData.push({ key: 'aggregateType', value: aggregateType });
	}
	if (entityId) {
		contextData.push({ key: 'entityId', value: entityId });
	}

	const newEvent: NewEvent = {
		id: event.eventId,
		specVersion: event.specVersion,
		type: event.eventType,
		source: event.source,
		subject: event.subject,
		time: event.time,
		data: JSON.parse(event.toDataJson()),
		correlationId: event.correlationId,
		causationId: event.causationId,
		deduplicationId: `${event.eventType}-${event.eventId}`,
		messageGroup: event.messageGroup,
		clientId,
		contextData: contextData.length > 0 ? contextData : null,
	};

	await db.insert(events).values(newEvent);
}

/**
 * Create an audit log record in the database.
 */
async function createAuditLogRecord(db: PostgresJsDatabase, event: DomainEvent, command: unknown): Promise<void> {
	const entityType = DomainEventUtils.extractAggregateType(event.subject);
	const entityId = DomainEventUtils.extractEntityId(event.subject);

	// Get operation name from command
	const operationName = getOperationName(command);

	const newAuditLog: NewAuditLog = {
		id: generate('AUDIT_LOG'),
		entityType,
		entityId: entityId ?? 'unknown',
		operation: operationName,
		operationJson: command !== null && command !== undefined ? JSON.parse(JSON.stringify(command)) : null,
		principalId: event.principalId,
		performedAt: event.time,
	};

	await db.insert(auditLogs).values(newAuditLog);
}

/**
 * Extract operation name from a command object.
 */
function getOperationName(command: unknown): string {
	if (command === null || command === undefined) {
		return 'Unknown';
	}

	// If it's a class instance, use the class name
	const constructor = (command as object).constructor;
	if (constructor && constructor.name && constructor.name !== 'Object') {
		return constructor.name;
	}

	// If it has an 'operation' or 'type' field, use that
	if (typeof command === 'object' && command !== null) {
		const cmd = command as Record<string, unknown>;
		if (typeof cmd['operation'] === 'string') {
			return cmd['operation'];
		}
		if (typeof cmd['type'] === 'string') {
			return cmd['type'];
		}
		if (typeof cmd['_type'] === 'string') {
			return cmd['_type'];
		}
	}

	return 'Unknown';
}

/**
 * No-op Unit of Work for testing.
 * Returns success without persisting anything.
 */
export function createNoOpUnitOfWork(): UnitOfWork {
	return {
		async commit<T extends DomainEvent>(_aggregate: Aggregate, event: T, _command: unknown): Promise<Result<T>> {
			return Result.success(RESULT_SUCCESS_TOKEN, event);
		},

		async commitDelete<T extends DomainEvent>(
			_aggregate: Aggregate,
			event: T,
			_command: unknown,
		): Promise<Result<T>> {
			return Result.success(RESULT_SUCCESS_TOKEN, event);
		},

		async commitAll<T extends DomainEvent>(
			_aggregates: Aggregate[],
			event: T,
			_command: unknown,
		): Promise<Result<T>> {
			return Result.success(RESULT_SUCCESS_TOKEN, event);
		},
	};
}
