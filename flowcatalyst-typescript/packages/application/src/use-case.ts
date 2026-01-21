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

import type { Result, DomainEvent, ExecutionContext } from '@flowcatalyst/domain-core';
import type { Command } from './command.js';

/**
 * UseCase interface for write operations.
 *
 * @typeParam TCommand - The command type (input data)
 * @typeParam TEvent - The domain event type (output on success)
 */
export interface UseCase<TCommand extends Command, TEvent extends DomainEvent> {
	/**
	 * Execute the use case.
	 *
	 * @param command - The input command with operation data
	 * @param context - Execution context with tracing and principal information
	 * @returns Result containing the domain event on success, or an error on failure
	 */
	execute(command: TCommand, context: ExecutionContext): Promise<Result<TEvent>>;
}

/**
 * Synchronous UseCase interface for operations that don't need async.
 * Prefer the async version for database operations.
 */
export interface SyncUseCase<TCommand extends Command, TEvent extends DomainEvent> {
	/**
	 * Execute the use case synchronously.
	 */
	execute(command: TCommand, context: ExecutionContext): Result<TEvent>;
}

/**
 * Type for extracting the command type from a UseCase.
 *
 * @example
 * ```typescript
 * type Cmd = UseCaseCommand<CreateUserUseCase>; // CreateUserCommand
 * ```
 */
export type UseCaseCommand<T> = T extends UseCase<infer TCommand, DomainEvent> ? TCommand : never;

/**
 * Type for extracting the event type from a UseCase.
 *
 * @example
 * ```typescript
 * type Evt = UseCaseEvent<CreateUserUseCase>; // UserCreatedEvent
 * ```
 */
export type UseCaseEvent<T> = T extends UseCase<Command, infer TEvent> ? TEvent : never;

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
export type UseCaseFactory<TCommand extends Command, TEvent extends DomainEvent, TDeps = unknown> = (
	deps: TDeps,
) => UseCase<TCommand, TEvent>;
