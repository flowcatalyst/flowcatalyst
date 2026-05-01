/**
 * SDK Error Types
 *
 * Discriminated union types for typed error handling with neverthrow.
 * All errors have a `type` field for pattern matching.
 */
/**
 * Create an authentication error
 */
export const authError = {
    missingCredentials: (message = "Client ID and secret are required") => ({
        type: "missing_credentials",
        message,
    }),
    invalidCredentials: (message = "Invalid client credentials") => ({
        type: "invalid_credentials",
        message,
    }),
    tokenExpired: (message = "Access token has expired") => ({
        type: "token_expired",
        message,
    }),
    tokenFetchFailed: (message, cause) => ({
        type: "token_fetch_failed",
        message,
        cause,
    }),
};
/**
 * Create an HTTP error
 */
export const httpError = {
    network: (message, cause) => ({
        type: "network",
        message,
        cause,
    }),
    timeout: (durationMs) => ({
        type: "timeout",
        message: `Request timed out after ${durationMs}ms`,
        durationMs,
    }),
    http: (status, message, body) => ({
        type: "http_error",
        status,
        message,
        body,
    }),
};
/**
 * Create a validation error from API response
 */
export const validationError = (message, errors = {}) => ({
    type: "validation",
    message,
    errors,
});
/**
 * Create a not found error
 */
export const notFoundError = (message, resource, id) => ({
    type: "not_found",
    message,
    resource,
    id,
});
/**
 * Create a forbidden error
 */
export const forbiddenError = (message = "Access forbidden") => ({
    type: "forbidden",
    message,
});
/**
 * Create a conflict error
 */
export const conflictError = (message, code) => ({
    type: "conflict",
    message,
    code,
});
/**
 * Create a rate limit error
 */
export const rateLimitError = (message, retryAfterMs) => ({
    type: "rate_limited",
    message,
    retryAfterMs,
});
/**
 * Map HTTP status code to appropriate error
 */
export function mapHttpStatusToError(status, body, message) {
    const errorBody = body;
    // Platform JSON: { error: "<CODE>", message: "<human text>" }
    // Prefer the human message; fall back to the code, then a generic.
    const errorMessage = message ??
        errorBody?.["message"]?.toString() ??
        errorBody?.["error"]?.toString() ??
        `HTTP ${status}`;
    const errorCode = errorBody?.["error"]?.toString();
    switch (status) {
        case 401:
            return authError.tokenExpired(errorMessage);
        case 403:
            return forbiddenError(errorMessage);
        case 404:
            return notFoundError(errorMessage);
        case 409:
            return conflictError(errorMessage, errorCode);
        case 422:
            return validationError(errorMessage, errorBody?.["errors"] ?? {});
        case 429: {
            const retryAfter = errorBody?.["retryAfter"];
            return rateLimitError(errorMessage, retryAfter ? retryAfter * 1000 : undefined);
        }
        default:
            return httpError.http(status, errorMessage, body);
    }
}
