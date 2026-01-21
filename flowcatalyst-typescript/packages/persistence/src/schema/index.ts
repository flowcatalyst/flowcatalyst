/**
 * Schema Exports
 *
 * All database schema definitions for the FlowCatalyst platform.
 */

// Common schema utilities
export { tsidColumn, rawTsidColumn, timestampColumn, baseEntityColumns, type BaseEntity, type NewEntity } from './common.js';

// Events schema
export { events, type Event, type NewEvent, type EventContextData } from './events.js';

// Events read schema (CQRS projection)
export { eventsRead, type EventReadRecord, type NewEventReadRecord } from './events-read.js';

// Audit logs schema
export { auditLogs, type AuditLogRecord, type NewAuditLog } from './audit-logs.js';

// Dispatch jobs schema
export {
	dispatchJobs,
	type DispatchJobRecord,
	type NewDispatchJobRecord,
	type DispatchKind,
	type DispatchProtocol,
	type DispatchStatus,
	type DispatchMode,
	type DispatchJobMetadata,
} from './dispatch-jobs.js';

// Dispatch jobs read schema (CQRS projection)
export { dispatchJobsRead, type DispatchJobReadRecord, type NewDispatchJobReadRecord } from './dispatch-jobs-read.js';

// Dispatch job attempts schema
export {
	dispatchJobAttempts,
	type DispatchJobAttemptRecord,
	type NewDispatchJobAttemptRecord,
	type DispatchErrorType,
} from './dispatch-job-attempts.js';

// Outbox schemas (CQRS change capture)
export {
	eventOutbox,
	dispatchJobOutbox,
	type EventOutboxRecord,
	type NewEventOutboxRecord,
	type DispatchJobOutboxRecord,
	type NewDispatchJobOutboxRecord,
	type OutboxOperation,
	type OutboxProcessedStatus,
} from './outbox.js';
