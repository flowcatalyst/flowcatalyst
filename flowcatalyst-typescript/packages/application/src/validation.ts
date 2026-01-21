/**
 * Validation Utilities
 *
 * Helper functions for common validation patterns in use cases.
 * All validation functions return Result types for consistent error handling.
 *
 * @example
 * ```typescript
 * // In a use case:
 * const emailResult = validateRequired(command.email, 'email', 'EMAIL_REQUIRED');
 * if (Result.isFailure(emailResult)) return emailResult;
 *
 * const formatResult = validateFormat(command.code, CODE_PATTERN, 'code', 'INVALID_CODE_FORMAT');
 * if (Result.isFailure(formatResult)) return formatResult;
 * ```
 */

import { Result, UseCaseError } from '@flowcatalyst/domain-core';

/**
 * Validate that a value is not null, undefined, or empty string.
 *
 * @param value - The value to validate
 * @param fieldName - The field name for error details
 * @param errorCode - The error code if validation fails
 * @param errorMessage - Optional custom error message
 * @returns Success with the value, or Failure with validation error
 */
export function validateRequired<T>(
	value: T | null | undefined,
	fieldName: string,
	errorCode: string,
	errorMessage?: string,
): Result<NonNullable<T>> {
	if (value === null || value === undefined) {
		return Result.failure(
			UseCaseError.validation(errorCode, errorMessage ?? `${fieldName} is required`, { field: fieldName }),
		);
	}

	if (typeof value === 'string' && value.trim() === '') {
		return Result.failure(
			UseCaseError.validation(errorCode, errorMessage ?? `${fieldName} is required`, { field: fieldName }),
		);
	}

	return unsafeSuccess(value as NonNullable<T>);
}

/**
 * Validate that a string matches a pattern.
 *
 * @param value - The string to validate
 * @param pattern - The regex pattern to match
 * @param fieldName - The field name for error details
 * @param errorCode - The error code if validation fails
 * @param errorMessage - Optional custom error message
 * @returns Success with the value, or Failure with validation error
 */
export function validateFormat(
	value: string,
	pattern: RegExp,
	fieldName: string,
	errorCode: string,
	errorMessage?: string,
): Result<string> {
	if (!pattern.test(value)) {
		return Result.failure(
			UseCaseError.validation(errorCode, errorMessage ?? `${fieldName} has invalid format`, {
				field: fieldName,
				pattern: pattern.source,
			}),
		);
	}

	return unsafeSuccess(value);
}

/**
 * Validate that a string does not exceed a maximum length.
 *
 * @param value - The string to validate
 * @param maxLength - The maximum allowed length
 * @param fieldName - The field name for error details
 * @param errorCode - The error code if validation fails
 * @returns Success with the value, or Failure with validation error
 */
export function validateMaxLength(
	value: string,
	maxLength: number,
	fieldName: string,
	errorCode: string,
): Result<string> {
	if (value.length > maxLength) {
		return Result.failure(
			UseCaseError.validation(errorCode, `${fieldName} must be ${maxLength} characters or less`, {
				field: fieldName,
				length: value.length,
				maxLength,
			}),
		);
	}

	return unsafeSuccess(value);
}

/**
 * Validate that a string meets a minimum length.
 *
 * @param value - The string to validate
 * @param minLength - The minimum required length
 * @param fieldName - The field name for error details
 * @param errorCode - The error code if validation fails
 * @returns Success with the value, or Failure with validation error
 */
export function validateMinLength(
	value: string,
	minLength: number,
	fieldName: string,
	errorCode: string,
): Result<string> {
	if (value.length < minLength) {
		return Result.failure(
			UseCaseError.validation(errorCode, `${fieldName} must be at least ${minLength} characters`, {
				field: fieldName,
				length: value.length,
				minLength,
			}),
		);
	}

	return unsafeSuccess(value);
}

/**
 * Validate that a value is within a numeric range.
 *
 * @param value - The number to validate
 * @param min - The minimum allowed value
 * @param max - The maximum allowed value
 * @param fieldName - The field name for error details
 * @param errorCode - The error code if validation fails
 * @returns Success with the value, or Failure with validation error
 */
export function validateRange(
	value: number,
	min: number,
	max: number,
	fieldName: string,
	errorCode: string,
): Result<number> {
	if (value < min || value > max) {
		return Result.failure(
			UseCaseError.validation(errorCode, `${fieldName} must be between ${min} and ${max}`, {
				field: fieldName,
				value,
				min,
				max,
			}),
		);
	}

	return unsafeSuccess(value);
}

/**
 * Validate that a value is one of the allowed values.
 *
 * @param value - The value to validate
 * @param allowedValues - Array of allowed values
 * @param fieldName - The field name for error details
 * @param errorCode - The error code if validation fails
 * @returns Success with the value, or Failure with validation error
 */
export function validateOneOf<T>(
	value: T,
	allowedValues: readonly T[],
	fieldName: string,
	errorCode: string,
): Result<T> {
	if (!allowedValues.includes(value)) {
		return Result.failure(
			UseCaseError.validation(errorCode, `${fieldName} must be one of: ${allowedValues.join(', ')}`, {
				field: fieldName,
				value,
				allowedValues,
			}),
		);
	}

	return unsafeSuccess(value);
}

/**
 * Validate an email address format.
 *
 * @param email - The email to validate
 * @param fieldName - The field name for error details (default: 'email')
 * @param errorCode - The error code if validation fails (default: 'INVALID_EMAIL')
 * @returns Success with the email, or Failure with validation error
 */
export function validateEmail(
	email: string,
	fieldName: string = 'email',
	errorCode: string = 'INVALID_EMAIL',
): Result<string> {
	// Basic email pattern - not meant to be exhaustive, just catch obvious errors
	const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

	if (!emailPattern.test(email)) {
		return Result.failure(UseCaseError.validation(errorCode, 'Invalid email format', { field: fieldName, email }));
	}

	return unsafeSuccess(email);
}

/**
 * Chain multiple validations together.
 * Stops at the first failure.
 *
 * @param validations - Array of validation functions to run
 * @returns Success if all validations pass, or the first Failure
 *
 * @example
 * ```typescript
 * const result = validateAll(
 *     () => validateRequired(command.name, 'name', 'NAME_REQUIRED'),
 *     () => validateMaxLength(command.name, 100, 'name', 'NAME_TOO_LONG'),
 *     () => validateEmail(command.email),
 * );
 * if (Result.isFailure(result)) return result;
 * ```
 */
export function validateAll(...validations: Array<() => Result<unknown>>): Result<void> {
	for (const validation of validations) {
		const result = validation();
		if (Result.isFailure(result)) {
			return Result.failure(result.error);
		}
	}

	return unsafeSuccess(undefined);
}

/**
 * Internal helper to create success results for validation.
 * This is safe because validations don't create state changes -
 * they just verify input before the actual use case logic runs.
 *
 * NOTE: This bypasses the Result.success() restriction because
 * validation is pre-UnitOfWork logic. The actual success result
 * still comes from UnitOfWork.commit().
 */
function unsafeSuccess<T>(value: T): Result<T> {
	return { _tag: 'success', value } as Result<T>;
}
