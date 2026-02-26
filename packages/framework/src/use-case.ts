/**
 * UseCase Interface
 *
 * UseCases encapsulate a single business operation. Each use case:
 * - Takes a command (input data) and execution context (tracing/principal)
 * - Performs validation and business rule checks
 * - Creates or modifies aggregates
 * - Returns a Result containing a domain event on success
 *
 * Key Constraint: UseCases can ONLY return success through UnitOfWork.commit(),
 * which guarantees domain events and audit logs are always created.
 *
 * @example
 * ```typescript
 * class CreateUserUseCase implements UseCase<CreateUserCommand, UserCreatedEvent> {
 *     constructor(
 *         private principalRepo: PrincipalRepository,
 *         private unitOfWork: UnitOfWork,
 *     ) {}
 *
 *     async execute(
 *         command: CreateUserCommand,
 *         context: ExecutionContext,
 *     ): Promise<Result<UserCreatedEvent>> {
 *         // 1. Validation
 *         if (!isValidEmail(command.email)) {
 *             return Result.failure(UseCaseError.validation('INVALID_EMAIL', 'Invalid email format'));
 *         }
 *
 *         // 2. Business rules
 *         const existing = await this.principalRepo.findByEmail(command.email);
 *         if (existing) {
 *             return Result.failure(UseCaseError.businessRule('EMAIL_EXISTS', 'Email already registered'));
 *         }
 *
 *         // 3. Create aggregate
 *         const principal = createPrincipal(command);
 *
 *         // 4. Create domain event
 *         const event = new UserCreatedEvent(context, { userId: principal.id, email: command.email });
 *
 *         // 5. Commit atomically (ONLY way to return success)
 *         return this.unitOfWork.commit(principal, event, command);
 *     }
 * }
 * ```
 */

import {
	Result,
	UseCaseError,
	type DomainEvent,
	type ExecutionContext,
} from "@flowcatalyst/domain";
import type { Command } from "./command.js";

export interface UseCase<TCommand extends Command, TEvent extends DomainEvent> {
	execute(
		command: TCommand,
		context: ExecutionContext,
	): Promise<Result<TEvent>>;
}

/**
 * Synchronous UseCase interface for operations that don't need async.
 * Prefer the async version for database operations.
 */
export interface SyncUseCase<
	TCommand extends Command,
	TEvent extends DomainEvent,
> {
	execute(command: TCommand, context: ExecutionContext): Result<TEvent>;
}

export type UseCaseCommand<T> =
	T extends UseCase<infer TCommand, DomainEvent> ? TCommand : never;

export type UseCaseEvent<T> =
	T extends UseCase<Command, infer TEvent> ? TEvent : never;

/**
 * A use case factory function type.
 * Use this when creating use cases with dependency injection.
 *
 * @example
 * ```typescript
 * const createUserUseCase: UseCaseFactory<CreateUserCommand, UserCreatedEvent> =
 *     (deps) => ({
 *         execute: async (command, context) => {
 *             // Implementation using deps
 *         },
 *     });
 * ```
 */
/**
 * Base class for use cases with resource-level authorization.
 *
 * Provides a template method via execute() that enforces resource-level
 * authorization before delegating to business logic.
 *
 * **Deny by default**: authorizeResource() returns false unless explicitly
 * overridden. Every use case must make a conscious authorization decision.
 * Use cases with no resource-level restriction must override and return true.
 *
 * The two-level authorization model:
 * 1. Action-level (API layer): "Can this principal perform this action?"
 * 2. Resource-level (use case layer): "Can this principal perform this
 *    action on THIS specific resource?" — handled by authorizeResource()
 *
 * @example
 * ```typescript
 * // Use case WITH resource restriction
 * class DeleteEventTypeUseCase extends SecuredUseCase<DeleteEventTypeCommand, EventTypeDeletedEvent> {
 *     authorizeResource(command: DeleteEventTypeCommand, context: ExecutionContext): boolean {
 *         const authz = context.authz;
 *         if (!authz) return true; // system call
 *         return AuthorizationContext.canAccessResourceWithPrefix(authz, command.code);
 *     }
 *
 *     async doExecute(command: DeleteEventTypeCommand, context: ExecutionContext): Promise<Result<EventTypeDeletedEvent>> {
 *         // Business logic here
 *     }
 * }
 *
 * // Use case WITHOUT resource restriction (must explicitly opt out)
 * class ListApplicationsUseCase extends SecuredUseCase<ListApplicationsCommand, ApplicationsListedEvent> {
 *     authorizeResource(): boolean {
 *         return true; // No resource-level restriction — filtering done in query
 *     }
 *     // ...
 * }
 * ```
 */
export abstract class SecuredUseCase<
	TCommand extends Command,
	TEvent extends DomainEvent,
> implements UseCase<TCommand, TEvent>
{
	async execute(
		command: TCommand,
		context: ExecutionContext,
	): Promise<Result<TEvent>> {
		if (!this.authorizeResource(command, context)) {
			return Result.failure(
				UseCaseError.authorization(
					"RESOURCE_ACCESS_DENIED",
					"Not authorized to access this resource",
				),
			);
		}
		return this.doExecute(command, context);
	}

	/**
	 * Resource-level authorization guard.
	 *
	 * **Returns false by default** — denies access unless explicitly overridden.
	 * Subclasses MUST override this method to either:
	 * - Return true (no resource-level restriction needed)
	 * - Check context.authz and return true/false based on the principal's access
	 *
	 * When authz is null (system/internal call), return true to bypass auth.
	 * When entity not found, return true to let doExecute() handle 404 properly.
	 */
	authorizeResource(_command: TCommand, _context: ExecutionContext): boolean {
		return false;
	}

	/**
	 * Business logic implementation.
	 */
	abstract doExecute(
		command: TCommand,
		context: ExecutionContext,
	): Promise<Result<TEvent>>;
}

export type UseCaseFactory<
	TCommand extends Command,
	TEvent extends DomainEvent,
	TDeps = unknown,
> = (deps: TDeps) => UseCase<TCommand, TEvent>;
