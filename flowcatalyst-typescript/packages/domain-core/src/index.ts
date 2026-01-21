/**
 * @flowcatalyst/domain-core
 *
 * Core domain infrastructure for the FlowCatalyst platform:
 * - Result type with restricted success factory
 * - Use case error types with HTTP status mapping
 * - Execution context for distributed tracing
 * - Tracing context with AsyncLocalStorage
 * - Audit context for principal tracking
 * - Domain event interface (CloudEvents-based)
 * - Unit of Work pattern for atomic commits
 * - Audit log types
 *
 * @example
 * ```typescript
 * import {
 *     Result,
 *     UseCaseError,
 *     ExecutionContext,
 *     TracingContext,
 *     AuditContext,
 *     DomainEvent,
 *     BaseDomainEvent,
 *     UnitOfWork,
 * } from '@flowcatalyst/domain-core';
 *
 * // Create execution context
 * const ctx = ExecutionContext.create(principalId);
 *
 * // Return failures directly
 * if (!isValid(input)) {
 *     return Result.failure(UseCaseError.validation('INVALID', 'Invalid input'));
 * }
 *
 * // Create domain event
 * const event = new MyDomainEvent(ctx, data);
 *
 * // Commit atomically through UnitOfWork (only way to return success)
 * return unitOfWork.commit(aggregate, event, command);
 * ```
 */

// Error types
export {
	UseCaseError,
	type UseCaseErrorBase,
	type ValidationError,
	type NotFoundError,
	type BusinessRuleViolation,
	type ConcurrencyError,
} from './errors.js';

// Result type
export {
	Result,
	isSuccess,
	isFailure,
	type Success,
	type Failure,
	// Internal exports for UnitOfWork implementations
	RESULT_SUCCESS_TOKEN,
	type ResultSuccessToken,
} from './result.js';

// Tracing context
export { TracingContext, type TracingContextData } from './tracing-context.js';

// Execution context
export { ExecutionContext, type ExecutionContext as ExecutionContextType } from './execution-context.js';

// Audit context
export {
	AuditContext,
	SYSTEM_PRINCIPAL_CODE,
	SYSTEM_PRINCIPAL_NAME,
	type AuditContextData,
	type PrincipalInfo,
	type PrincipalType,
	type UserScope,
} from './audit-context.js';

// Domain events
export {
	DomainEvent,
	BaseDomainEvent,
	type DomainEvent as DomainEventType,
	type DomainEventBase,
	type DomainEventMetadata,
} from './domain-event.js';

// Audit log
export { type AuditLog, type CreateAuditLogData } from './audit-log.js';

// Unit of Work
export { type UnitOfWork, type Aggregate } from './unit-of-work.js';
