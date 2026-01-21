// Package common provides core infrastructure for the UseCase/UnitOfWork pattern.
// This package enforces that all successful mutations go through UnitOfWork.Commit(),
// guaranteeing that domain events and audit logs are always emitted.
package common

// Result represents the outcome of a use case execution.
// Success can ONLY be created via UnitOfWork.Commit() to guarantee
// domain events and audit logs are always emitted.
//
// The success constructor (newSuccess) is unexported, meaning only code within
// this package (specifically MongoUnitOfWork) can create successful results.
// This is the key enforcement mechanism that guarantees every successful
// mutation emits a domain event and creates an audit log.
type Result[T any] struct {
	value   T
	err     *UseCaseError
	success bool
}

// newSuccess creates a successful result.
// This is unexported - only UnitOfWork.Commit() can create successful results,
// ensuring domain events are always emitted.
func newSuccess[T any](value T) Result[T] {
	return Result[T]{
		value:   value,
		success: true,
	}
}

// Failure creates a failed result.
// This is public - any code can return failures for validation errors,
// business rule violations, etc. without going through UnitOfWork.
func Failure[T any](err *UseCaseError) Result[T] {
	return Result[T]{
		err:     err,
		success: false,
	}
}

// FailureFrom creates a failed result from an existing error.
// Convenience method for wrapping errors.
func FailureFrom[T any](err *UseCaseError) Result[T] {
	return Failure[T](err)
}

// IsSuccess returns true if the result is successful.
func (r Result[T]) IsSuccess() bool {
	return r.success
}

// IsFailure returns true if the result is a failure.
func (r Result[T]) IsFailure() bool {
	return !r.success
}

// Value returns the success value.
// Should only be called after checking IsSuccess().
func (r Result[T]) Value() T {
	return r.value
}

// Error returns the error if the result is a failure, nil otherwise.
func (r Result[T]) Error() *UseCaseError {
	return r.err
}

// Map transforms a successful result's value using the provided function.
// If the result is a failure, it returns the failure unchanged.
func Map[T, U any](r Result[T], fn func(T) U) Result[U] {
	if r.IsFailure() {
		return Failure[U](r.err)
	}
	return newSuccess(fn(r.value))
}

// FlatMap chains result-returning operations.
// If the result is a failure, it returns the failure unchanged.
// If successful, it applies the function which returns a new Result.
func FlatMap[T, U any](r Result[T], fn func(T) Result[U]) Result[U] {
	if r.IsFailure() {
		return Failure[U](r.err)
	}
	return fn(r.value)
}

// Match applies one of two functions depending on success/failure state.
// This is useful for handling both cases in a single expression.
func Match[T, U any](r Result[T], onSuccess func(T) U, onFailure func(*UseCaseError) U) U {
	if r.IsSuccess() {
		return onSuccess(r.value)
	}
	return onFailure(r.err)
}

// OrElse returns the success value or the provided default if failure.
func (r Result[T]) OrElse(defaultValue T) T {
	if r.IsSuccess() {
		return r.value
	}
	return defaultValue
}

// OrElseGet returns the success value or calls the provided function if failure.
func (r Result[T]) OrElseGet(fn func() T) T {
	if r.IsSuccess() {
		return r.value
	}
	return fn()
}
