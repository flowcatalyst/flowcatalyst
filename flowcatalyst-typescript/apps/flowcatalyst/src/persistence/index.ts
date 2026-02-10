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
export {
  createDatabase,
  createMigrationDatabase,
  type Database,
  type DatabaseConfig,
} from './connection.js';

// Migration runner
export { runMigrations } from './migrate.js';

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
  rawTsidColumn,
  timestampColumn,
  baseEntityColumns,
  type BaseEntity,
  type NewEntity,
  // Events
  events,
  type Event,
  type NewEvent,
  type EventContextData,
  // Events read (CQRS projection)
  eventsRead,
  type EventReadRecord,
  type NewEventReadRecord,
  // Audit logs
  auditLogs,
  type AuditLogRecord,
  type NewAuditLog,
  // Dispatch jobs
  dispatchJobs,
  type DispatchJobRecord,
  type NewDispatchJobRecord,
  type DispatchKind,
  type DispatchProtocol,
  type DispatchStatus,
  type DispatchMode,
  type DispatchJobMetadata,
  // Dispatch jobs read (CQRS projection)
  dispatchJobsRead,
  type DispatchJobReadRecord,
  type NewDispatchJobReadRecord,
  // Dispatch job attempts
  dispatchJobAttempts,
  type DispatchJobAttemptRecord,
  type NewDispatchJobAttemptRecord,
  type DispatchErrorType,
  // Projection feed
  eventProjectionFeed,
  dispatchJobProjectionFeed,
  type EventProjectionFeedRecord,
  type NewEventProjectionFeedRecord,
  type DispatchJobProjectionFeedRecord,
  type NewDispatchJobProjectionFeedRecord,
  type ProjectionFeedOperation,
  type ProjectionFeedProcessedStatus,
} from './schema/index.js';
