/**
 * Operations Service Pattern
 *
 * Operations services provide a facade over related use cases, offering:
 * - A single entry point for all mutations on an aggregate type
 * - Clear separation of read (queries) and write (commands) operations
 * - Consistent interface for HTTP handlers to call
 *
 * @example
 * ```typescript
 * class UserOperations {
 *     constructor(
 *         private createUserUseCase: CreateUserUseCase,
 *         private updateUserUseCase: UpdateUserUseCase,
 *         private deleteUserUseCase: DeleteUserUseCase,
 *         private userRepository: UserRepository,
 *     ) {}
 *
 *     // Write operations - delegate to use cases
 *     createUser(command: CreateUserCommand, context: ExecutionContext) {
 *         return this.createUserUseCase.execute(command, context);
 *     }
 *
 *     updateUser(command: UpdateUserCommand, context: ExecutionContext) {
 *         return this.updateUserUseCase.execute(command, context);
 *     }
 *
 *     // Read operations - call repositories directly
 *     findById(id: string): User | null {
 *         return this.userRepository.findById(id);
 *     }
 *
 *     findByEmail(email: string): User | null {
 *         return this.userRepository.findByEmail(email);
 *     }
 * }
 * ```
 */

import type { Result, DomainEvent, ExecutionContext } from '@flowcatalyst/domain-core';
import type { Command } from './command.js';
import type { UseCase } from './use-case.js';

/**
 * Type for a write operation function in an operations service.
 */
export type WriteOperation<TCommand extends Command, TEvent extends DomainEvent> = (
	command: TCommand,
	context: ExecutionContext,
) => Promise<Result<TEvent>>;

/**
 * Type for a read operation function in an operations service.
 */
export type ReadOperation<TResult, TParams extends unknown[] = []> = (...params: TParams) => Promise<TResult>;

/**
 * Type for a synchronous read operation function.
 */
export type SyncReadOperation<TResult, TParams extends unknown[] = []> = (...params: TParams) => TResult;

/**
 * Helper to create a write operation from a use case.
 * Provides a cleaner interface when building operations services.
 *
 * @param useCase - The use case to wrap
 * @returns A write operation function
 *
 * @example
 * ```typescript
 * const operations = {
 *     createUser: wrapUseCase(createUserUseCase),
 *     updateUser: wrapUseCase(updateUserUseCase),
 * };
 * ```
 */
export function wrapUseCase<TCommand extends Command, TEvent extends DomainEvent>(
	useCase: UseCase<TCommand, TEvent>,
): WriteOperation<TCommand, TEvent> {
	return (command, context) => useCase.execute(command, context);
}

/**
 * Builder for creating operations services with type safety.
 *
 * @example
 * ```typescript
 * const userOperations = createOperationsService()
 *     .write('createUser', createUserUseCase)
 *     .write('updateUser', updateUserUseCase)
 *     .write('deleteUser', deleteUserUseCase)
 *     .read('findById', (id: string) => userRepo.findById(id))
 *     .read('findByEmail', (email: string) => userRepo.findByEmail(email))
 *     .build();
 *
 * // Usage:
 * const result = await userOperations.createUser(command, context);
 * const user = await userOperations.findById('user-123');
 * ```
 */
export function createOperationsService(): OperationsBuilder<object> {
	return new OperationsBuilder({});
}

/**
 * Builder class for operations services.
 */
class OperationsBuilder<T extends object> {
	constructor(private readonly ops: T) {}

	/**
	 * Add a write operation backed by a use case.
	 */
	write<TName extends string, TCommand extends Command, TEvent extends DomainEvent>(
		name: TName,
		useCase: UseCase<TCommand, TEvent>,
	): OperationsBuilder<T & { [K in TName]: WriteOperation<TCommand, TEvent> }> {
		const newOps = { ...this.ops, [name]: wrapUseCase(useCase) } as T & {
			[K in TName]: WriteOperation<TCommand, TEvent>;
		};
		return new OperationsBuilder(newOps);
	}

	/**
	 * Add an async read operation.
	 */
	read<TName extends string, TResult, TParams extends unknown[]>(
		name: TName,
		operation: (...params: TParams) => Promise<TResult>,
	): OperationsBuilder<T & { [K in TName]: (...params: TParams) => Promise<TResult> }> {
		const newOps = { ...this.ops, [name]: operation } as T & {
			[K in TName]: (...params: TParams) => Promise<TResult>;
		};
		return new OperationsBuilder(newOps);
	}

	/**
	 * Add a synchronous read operation.
	 */
	syncRead<TName extends string, TResult, TParams extends unknown[]>(
		name: TName,
		operation: (...params: TParams) => TResult,
	): OperationsBuilder<T & { [K in TName]: (...params: TParams) => TResult }> {
		const newOps = { ...this.ops, [name]: operation } as T & {
			[K in TName]: (...params: TParams) => TResult;
		};
		return new OperationsBuilder(newOps);
	}

	/**
	 * Build the operations service.
	 */
	build(): T {
		return this.ops;
	}
}

/**
 * Type helper to extract the operations type from a builder.
 */
export type OperationsType<T extends OperationsBuilder<object>> = T extends OperationsBuilder<infer U> ? U : never;
