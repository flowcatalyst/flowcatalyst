package tech.flowcatalyst.platform.common.errors;

import java.util.Map;

/**
 * Sealed error hierarchy for use case failures.
 *
 * Errors are categorized by type to enable consistent HTTP status mapping
 * and client-side handling.
 */
public sealed interface UseCaseError {

    String code();
    String message();
    Map<String, Object> details();

    /**
     * Input validation failed (missing required fields, invalid format, etc.)
     * Maps to HTTP 400 Bad Request.
     */
    record ValidationError(
        String code,
        String message,
        Map<String, Object> details
    ) implements UseCaseError {}

    /**
     * Business rule violation (entity in wrong state, constraint violated, etc.)
     * Maps to HTTP 409 Conflict.
     */
    record BusinessRuleViolation(
        String code,
        String message,
        Map<String, Object> details
    ) implements UseCaseError {}

    /**
     * Entity not found.
     * Maps to HTTP 404 Not Found.
     */
    record NotFoundError(
        String code,
        String message,
        Map<String, Object> details
    ) implements UseCaseError {}

    /**
     * Optimistic locking conflict - entity was modified by another transaction.
     * Maps to HTTP 409 Conflict.
     */
    record ConcurrencyError(
        String code,
        String message,
        Map<String, Object> details
    ) implements UseCaseError {}

    /**
     * Authorization failed - principal not authorized to perform this action.
     * Maps to HTTP 403 Forbidden.
     */
    record AuthorizationError(
        String code,
        String message,
        Map<String, Object> details
    ) implements UseCaseError {}
}
