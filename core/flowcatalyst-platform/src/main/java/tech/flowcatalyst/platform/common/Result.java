package tech.flowcatalyst.platform.common;

import tech.flowcatalyst.platform.common.errors.UseCaseError;

/**
 * Result type for use case execution.
 *
 * <p>This is a sealed interface with two variants:
 * <ul>
 *   <li>{@link Success} - contains the successful result value</li>
 *   <li>{@link Failure} - contains the error details</li>
 * </ul>
 *
 * <p><strong>Important:</strong> The {@link #success(Object)} factory method is
 * package-private. This ensures that only {@link UnitOfWork} can create successful
 * results, guaranteeing that domain events are always emitted when state changes.
 *
 * <p>Usage in use cases:
 * <pre>{@code
 * // Return failure for validation/business rule violations
 * if (!isValid) {
 *     return Result.failure(new ValidationError(...));
 * }
 *
 * // Return success only through UnitOfWork.commit()
 * return unitOfWork.commit(aggregate, event, command);
 * }</pre>
 *
 * <p>Usage in API layer:
 * <pre>{@code
 * return switch (result) {
 *     case Result.Success<MyEvent> s -> Response.ok(s.value()).build();
 *     case Result.Failure<MyEvent> f -> Response.status(400).entity(f.error()).build();
 * };
 * }</pre>
 */
public sealed interface Result<T> permits Result.Success, Result.Failure {

    boolean isSuccess();
    boolean isFailure();

    /**
     * Successful result containing the value.
     */
    record Success<T>(T value) implements Result<T> {
        @Override
        public boolean isSuccess() {
            return true;
        }

        @Override
        public boolean isFailure() {
            return false;
        }
    }

    /**
     * Failed result containing the error.
     */
    record Failure<T>(UseCaseError error) implements Result<T> {
        @Override
        public boolean isSuccess() {
            return false;
        }

        @Override
        public boolean isFailure() {
            return true;
        }
    }

    /**
     * Create a successful result.
     *
     * <p><strong>Package-private:</strong> Only {@link UnitOfWork} can create
     * successful results, ensuring domain events are always emitted.
     */
    static <T> Result<T> success(T value) {
        return new Success<>(value);
    }

    /**
     * Create a failed result.
     *
     * <p>This is public - any code can create failures for validation
     * errors, business rule violations, etc.
     */
    static <T> Result<T> failure(UseCaseError error) {
        return new Failure<>(error);
    }
}
