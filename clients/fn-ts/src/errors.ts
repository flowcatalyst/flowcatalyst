// Error codes shared across every host op (plan §5.3).

export const ErrCodeNotDeclared = "NOT_DECLARED";
export const ErrCodeNotAllowed = "NOT_ALLOWED";
export const ErrCodeDeadline = "DEADLINE";
export const ErrCodeTooLarge = "TOO_LARGE";
export const ErrCodeUnavailable = "UNAVAILABLE";
export const ErrCodeBadRequest = "BAD_REQUEST";
export const ErrCodeCapabilityUnavailable = "CAPABILITY_UNAVAILABLE";
export const ErrCodeDBConstraint = "DB_CONSTRAINT";
export const ErrCodeDBSyntax = "DB_SYNTAX";
export const ErrCodeDBTimeout = "DB_TIMEOUT";
export const ErrCodeDBUnavailable = "DB_UNAVAILABLE";
export const ErrCodeDBError = "DB_ERROR";
export const ErrCodeDBTxUnknown = "DB_TX_UNKNOWN";

/**
 * FnError is thrown when a host capability reports failure: `code` is one of
 * the ErrCode* constants above (plan §5.3), `message` is the host's
 * free-text detail. `retryable` is true exactly when `code` is
 * `UNAVAILABLE` (mirrors the Go SDK's `ErrRetryable` / the Rust SDK's
 * `HostError::is_retryable`).
 */
export class FnError extends Error {
  readonly code: string;
  readonly retryable: boolean;

  constructor(code: string, message: string, retryable = code === ErrCodeUnavailable) {
    super(message ? `fn: ${code}: ${message}` : `fn: ${code}`);
    this.name = "FnError";
    this.code = code;
    this.retryable = retryable;
  }
}

/** isRetryable mirrors errors.Is(err, fn.ErrRetryable) from the Go SDK. */
export function isRetryable(err: unknown): boolean {
  return err instanceof FnError && err.retryable;
}
