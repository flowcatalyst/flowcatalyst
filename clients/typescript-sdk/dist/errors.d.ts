/**
 * SDK Error Types
 *
 * Discriminated union types for typed error handling with neverthrow.
 * All errors have a `type` field for pattern matching.
 */
/** Authentication errors */
export type AuthenticationError = {
    type: "missing_credentials";
    message: string;
} | {
    type: "invalid_credentials";
    message: string;
} | {
    type: "token_expired";
    message: string;
} | {
    type: "token_fetch_failed";
    message: string;
    cause?: Error;
};
/** HTTP/Network errors */
export type HttpError = {
    type: "network";
    message: string;
    cause?: Error;
} | {
    type: "timeout";
    message: string;
    durationMs: number;
} | {
    type: "http_error";
    status: number;
    message: string;
    body?: unknown;
};
/** Validation errors (422) */
export type ValidationError = {
    type: "validation";
    message: string;
    errors: Record<string, string[]>;
};
/** Not found errors (404) */
export type NotFoundError = {
    type: "not_found";
    message: string;
    resource?: string;
    id?: string;
};
/** Forbidden errors (403) */
export type ForbiddenError = {
    type: "forbidden";
    message: string;
};
/** Conflict errors (409) */
export type ConflictError = {
    type: "conflict";
    message: string;
    code?: string;
};
/** Rate limit errors (429) */
export type RateLimitError = {
    type: "rate_limited";
    message: string;
    retryAfterMs?: number;
};
/** Union of all SDK errors */
export type SdkError = AuthenticationError | HttpError | ValidationError | NotFoundError | ForbiddenError | ConflictError | RateLimitError;
/**
 * Create an authentication error
 */
export declare const authError: {
    missingCredentials: (message?: string) => AuthenticationError;
    invalidCredentials: (message?: string) => AuthenticationError;
    tokenExpired: (message?: string) => AuthenticationError;
    tokenFetchFailed: (message: string, cause?: Error) => AuthenticationError;
};
/**
 * Create an HTTP error
 */
export declare const httpError: {
    network: (message: string, cause?: Error) => HttpError;
    timeout: (durationMs: number) => HttpError;
    http: (status: number, message: string, body?: unknown) => HttpError;
};
/**
 * Create a validation error from API response
 */
export declare const validationError: (message: string, errors?: Record<string, string[]>) => ValidationError;
/**
 * Create a not found error
 */
export declare const notFoundError: (message: string, resource?: string, id?: string) => NotFoundError;
/**
 * Create a forbidden error
 */
export declare const forbiddenError: (message?: string) => ForbiddenError;
/**
 * Create a conflict error
 */
export declare const conflictError: (message: string, code?: string) => ConflictError;
/**
 * Create a rate limit error
 */
export declare const rateLimitError: (message: string, retryAfterMs?: number) => RateLimitError;
/**
 * Map HTTP status code to appropriate error
 */
export declare function mapHttpStatusToError(status: number, body?: unknown, message?: string): SdkError;
//# sourceMappingURL=errors.d.ts.map