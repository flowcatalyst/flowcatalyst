package tech.flowcatalyst.platform.common;

import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.function.Function;

/**
 * General-purpose disjoint union type.
 *
 * <p>By convention, {@link Left} represents the failure/error case and
 * {@link Right} represents the success case. This follows the Scala/Haskell
 * convention where "right" is "correct".
 *
 * <p>{@code Either} is useful for validation pipelines, error composition,
 * and any case where you need a typed union of two values without the
 * domain-specific semantics of {@link Result}.
 *
 * <p>To bridge to the use case layer, use {@link #toResult(Function)} to
 * convert an {@code Either<L, R>} into a {@code Result<R>}.
 *
 * <p>Usage:
 * <pre>{@code
 * Either<String, Integer> parsed = Either.right(42);
 *
 * // Transform the right value
 * Either<String, String> formatted = parsed.map(n -> "Value: " + n);
 *
 * // Chain operations that may fail
 * Either<String, Double> result = parsed
 *     .flatMap(n -> n > 0
 *         ? Either.right(Math.sqrt(n))
 *         : Either.left("Cannot sqrt negative"));
 *
 * // Extract with fold
 * String message = result.fold(
 *     error -> "Error: " + error,
 *     value -> "Result: " + value
 * );
 * }</pre>
 *
 * @param <L> The left (error) type
 * @param <R> The right (success) type
 */
public sealed interface Either<L, R> permits Either.Left, Either.Right {

    record Left<L, R>(L value) implements Either<L, R> {}
    record Right<L, R>(R value) implements Either<L, R> {}

    /**
     * Create a left (error) value.
     */
    static <L, R> Either<L, R> left(L value) {
        return new Left<>(value);
    }

    /**
     * Create a right (success) value.
     */
    static <L, R> Either<L, R> right(R value) {
        return new Right<>(value);
    }

    /**
     * Transform the right value, leaving left unchanged.
     */
    @SuppressWarnings("unchecked")
    default <T> Either<L, T> map(Function<R, T> fn) {
        return switch (this) {
            case Left<L, R> l -> (Either<L, T>) l;
            case Right<L, R> r -> new Right<>(fn.apply(r.value()));
        };
    }

    /**
     * Chain an operation that returns Either, leaving left unchanged.
     */
    @SuppressWarnings("unchecked")
    default <T> Either<L, T> flatMap(Function<R, Either<L, T>> fn) {
        return switch (this) {
            case Left<L, R> l -> (Either<L, T>) l;
            case Right<L, R> r -> fn.apply(r.value());
        };
    }

    /**
     * Reduce to a single value by handling both cases.
     */
    default <T> T fold(Function<L, T> onLeft, Function<R, T> onRight) {
        return switch (this) {
            case Left<L, R> l -> onLeft.apply(l.value());
            case Right<L, R> r -> onRight.apply(r.value());
        };
    }

    /**
     * Bridge to {@link Result} by mapping the left value to a {@link UseCaseError}.
     */
    default Result<R> toResult(Function<L, UseCaseError> errorMapper) {
        return switch (this) {
            case Left<L, R> l -> Result.failure(errorMapper.apply(l.value()));
            case Right<L, R> r -> Result.success(r.value());
        };
    }
}
