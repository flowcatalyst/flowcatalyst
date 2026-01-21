/**
 * Result Type for Use Case Execution
 *
 * This is a discriminated union with two variants:
 * - Success<T> - contains the successful result value
 * - Failure<T> - contains the error details
 *
 * **IMPORTANT:** The `success()` factory is restricted. Only UnitOfWork can create
 * successful results, guaranteeing that domain events are always emitted when
 * state changes.
 *
 * Usage in use cases:
 * ```typescript
 * // Return failure for validation/business rule violations
 * if (!isValid) {
 *     return Result.failure(UseCaseError.validation('INVALID', 'Invalid input'));
 * }
 *
 * // Return success only through UnitOfWork.commit()
 * return unitOfWork.commit(aggregate, event, command);
 * ```
 *
 * Usage in API layer:
 * ```typescript
 * if (result.isSuccess()) {
 *     return c.json(result.value, 200);
 * } else {
 *     return c.json(result.error, UseCaseError.httpStatus(result.error));
 * }
 * ```
 */

import type { UseCaseError } from './errors.js';

/**
 * Token for authorizing Result.success() creation.
 * Only UnitOfWork implementations should have access to this token.
 *
 * @internal
 */
export const RESULT_SUCCESS_TOKEN: unique symbol = Symbol('RESULT_SUCCESS_TOKEN');

/**
 * Type for the success authorization token.
 *
 * @internal
 */
export type ResultSuccessToken = typeof RESULT_SUCCESS_TOKEN;

/**
 * Successful result containing the value.
 */
export interface Success<T> {
	readonly _tag: 'success';
	readonly value: T;
}

/**
 * Failed result containing the error.
 */
export interface Failure<T> {
	readonly _tag: 'failure';
	readonly error: UseCaseError;
}

/**
 * Result type - either Success or Failure.
 */
export type Result<T> = Success<T> | Failure<T>;

/**
 * Type guard to check if a result is a success.
 */
export function isSuccess<T>(result: Result<T>): result is Success<T> {
	return result._tag === 'success';
}

/**
 * Type guard to check if a result is a failure.
 */
export function isFailure<T>(result: Result<T>): result is Failure<T> {
	return result._tag === 'failure';
}

/**
 * Result factory functions.
 */
export const Result = {
	/**
	 * Create a successful result.
	 *
	 * **RESTRICTED:** Requires the success token. Only UnitOfWork should call this.
	 * Use cases must return success through `unitOfWork.commit()`.
	 *
	 * @param token - The success authorization token (only available to UnitOfWork)
	 * @param value - The success value
	 * @returns A Success result
	 * @throws Error if token is invalid
	 *
	 * @internal
	 */
	success<T>(token: ResultSuccessToken, value: T): Success<T> {
		if (token !== RESULT_SUCCESS_TOKEN) {
			throw new Error(
				'Result.success() is restricted. Use UnitOfWork.commit() to create successful results. ' +
					'This ensures domain events and audit logs are always created with state changes.',
			);
		}
		return { _tag: 'success', value } as Success<T>;
	},

	/**
	 * Create a failed result.
	 *
	 * This is public - any code can create failures for validation errors,
	 * business rule violations, etc.
	 *
	 * @param error - The use case error
	 * @returns A Failure result
	 */
	failure<T>(error: UseCaseError): Failure<T> {
		return { _tag: 'failure', error } as Failure<T>;
	},

	/**
	 * Check if a result is a success.
	 */
	isSuccess,

	/**
	 * Check if a result is a failure.
	 */
	isFailure,

	/**
	 * Map a successful result to a new value.
	 *
	 * @param result - The result to map
	 * @param fn - The mapping function
	 * @returns A new result with the mapped value
	 */
	map<T, U>(result: Result<T>, fn: (value: T) => U): Result<U> {
		if (isSuccess(result)) {
			// Note: We create a new Success directly here since we're just transforming
			// an existing success, not creating one from a use case
			return { _tag: 'success', value: fn(result.value) } as Success<U>;
		}
		return { _tag: 'failure', error: result.error } as Failure<U>;
	},

	/**
	 * Match on a result, handling both success and failure cases.
	 *
	 * @param result - The result to match
	 * @param onSuccess - Handler for success case
	 * @param onFailure - Handler for failure case
	 * @returns The result of the matched handler
	 */
	match<T, U>(result: Result<T>, onSuccess: (value: T) => U, onFailure: (error: UseCaseError) => U): U {
		if (isSuccess(result)) {
			return onSuccess(result.value);
		}
		return onFailure(result.error);
	},

	/**
	 * Get the value from a success result, or throw an error.
	 *
	 * @param result - The result to unwrap
	 * @returns The success value
	 * @throws Error if the result is a failure
	 */
	unwrap<T>(result: Result<T>): T {
		if (isSuccess(result)) {
			return result.value;
		}
		throw new Error(`Cannot unwrap failure result: ${result.error.code} - ${result.error.message}`);
	},

	/**
	 * Get the value from a success result, or return a default.
	 *
	 * @param result - The result to unwrap
	 * @param defaultValue - The default value if result is a failure
	 * @returns The success value or the default
	 */
	unwrapOr<T>(result: Result<T>, defaultValue: T): T {
		if (isSuccess(result)) {
			return result.value;
		}
		return defaultValue;
	},
};
