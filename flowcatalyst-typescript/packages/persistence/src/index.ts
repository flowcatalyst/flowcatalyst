/**
 * @flowcatalyst/persistence
 *
 * Database persistence layer for the FlowCatalyst platform using DrizzleORM.
 *
 * Key components:
 * - Database connection and configuration
 * - Transaction management
 * - Repository interfaces and utilities
 * - Aggregate registry for entity dispatch
 * - DrizzleTransactionalUnitOfWork for atomic commits
 * - Schema definitions for events and audit logs
 *
 * @example
 * ```typescript
 * import {
 *     createDatabase,
 *     createTransactionManager,
 *     createAggregateRegistry,
 *     createDrizzleUnitOfWork,
 *     events,
 *     auditLogs,
 * } from '@flowcatalyst/persistence';
 *
 * // Setup database
 * const database = createDatabase({ url: process.env.DATABASE_URL });
 * const transactionManager = createTransactionManager(database.db);
 *
 * // Setup aggregate registry
 * const aggregateRegistry = createAggregateRegistry();
 * aggregateRegistry.register(createAggregateHandler('User', userRepository));
 *
 * // Create unit of work
 * const unitOfWork = createDrizzleUnitOfWork({
 *     transactionManager,
 *     aggregateRegistry,
 * });
 *
 * // Use in a use case
 * const result = await unitOfWork.commit(user, userCreatedEvent, createUserCommand);
 * ```
 */

// Database connection
export { createDatabase, createMigrationDatabase, type Database, type DatabaseConfig } from './connection.js';

// Transaction management
export {
	createTransactionManager,
	resolveDb,
	type TransactionContext,
	type TransactionManager,
} from './transaction.js';

// Repository interfaces
export {
	type Repository,
	type PaginatedRepository,
	type PagedResult,
	createPagedResult,
} from './repository.js';

// Aggregate registry
export {
	createAggregateRegistry,
	createAggregateHandler,
	tagAggregate,
	isTaggedAggregate,
	type AggregateRegistry,
	type AggregateHandler,
	type TaggedAggregate,
} from './aggregate-registry.js';

// Unit of Work
export {
	createDrizzleUnitOfWork,
	createNoOpUnitOfWork,
	type DrizzleUnitOfWorkConfig,
} from './unit-of-work.js';

// Schema definitions
export {
	// Common
	tsidColumn,
	timestampColumn,
	baseEntityColumns,
	type BaseEntity,
	type NewEntity,
	// Events
	events,
	type Event,
	type NewEvent,
	type EventContextData,
	// Audit logs
	auditLogs,
	type AuditLogRecord,
	type NewAuditLog,
} from './schema/index.js';
