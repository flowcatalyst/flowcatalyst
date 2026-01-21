/**
 * @flowcatalyst/application
 *
 * Application layer patterns for the FlowCatalyst platform:
 * - Command types for write operation inputs
 * - UseCase interfaces for business operations
 * - Validation utilities for input validation
 * - Operations services for aggregating related use cases
 *
 * @example
 * ```typescript
 * import {
 *     Command,
 *     UseCase,
 *     validateRequired,
 *     validateEmail,
 *     createOperationsService,
 * } from '@flowcatalyst/application';
 * import { Result, UseCaseError, ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';
 *
 * // Define a command
 * interface CreateUserCommand extends Command {
 *     readonly email: string;
 *     readonly password: string;
 *     readonly name: string;
 * }
 *
 * // Implement a use case
 * class CreateUserUseCase implements UseCase<CreateUserCommand, UserCreatedEvent> {
 *     constructor(
 *         private userRepo: UserRepository,
 *         private unitOfWork: UnitOfWork,
 *     ) {}
 *
 *     async execute(command: CreateUserCommand, context: ExecutionContext): Promise<Result<UserCreatedEvent>> {
 *         // Validation
 *         const emailResult = validateEmail(command.email);
 *         if (Result.isFailure(emailResult)) return emailResult;
 *
 *         // Business rules
 *         const existing = await this.userRepo.findByEmail(command.email);
 *         if (existing) {
 *             return Result.failure(UseCaseError.businessRule('EMAIL_EXISTS', 'Email already registered'));
 *         }
 *
 *         // Create aggregate and event
 *         const user = createUser(command);
 *         const event = new UserCreatedEvent(context, { userId: user.id, email: user.email });
 *
 *         // Atomic commit (only way to return success)
 *         return this.unitOfWork.commit(user, event, command);
 *     }
 * }
 *
 * // Create an operations service
 * const userOperations = createOperationsService()
 *     .write('createUser', createUserUseCase)
 *     .read('findById', (id: string) => userRepo.findById(id))
 *     .build();
 * ```
 */

// Command types
export { type Command, type PartialCommand, type EntityCommand, type DeleteCommand, createCommand } from './command.js';

// UseCase interfaces
export {
	type UseCase,
	type SyncUseCase,
	type UseCaseCommand,
	type UseCaseEvent,
	type UseCaseFactory,
} from './use-case.js';

// Validation utilities
export {
	validateRequired,
	validateFormat,
	validateMaxLength,
	validateMinLength,
	validateRange,
	validateOneOf,
	validateEmail,
	validateAll,
} from './validation.js';

// Operations service pattern
export {
	type WriteOperation,
	type ReadOperation,
	type SyncReadOperation,
	type OperationsType,
	wrapUseCase,
	createOperationsService,
} from './operations.js';

// Re-export commonly used types from domain-core for convenience
export {
	Result,
	isSuccess,
	isFailure,
	type Success,
	type Failure,
	UseCaseError,
	ExecutionContext,
	type DomainEvent,
	type UnitOfWork,
} from '@flowcatalyst/domain-core';
