/**
 * Login Attempt Entity
 *
 * Represents a single authentication attempt — either a password-based
 * user login or an OAuth client_credentials token request by a service account.
 */

/**
 * The kind of authentication attempt.
 */
export type LoginAttemptType = "USER_LOGIN" | "SERVICE_ACCOUNT_TOKEN";

/**
 * The outcome of the authentication attempt.
 */
export type LoginOutcome = "SUCCESS" | "FAILURE";

/**
 * The reason an authentication attempt failed.
 */
export type LoginFailureReason =
	| "INVALID_PASSWORD"
	| "USER_NOT_FOUND"
	| "USER_INACTIVE"
	| "NO_PASSWORD_SET"
	| "INVALID_CLIENT_SECRET"
	| "CLIENT_NOT_FOUND";

/**
 * A login attempt record.
 */
export interface LoginAttempt {
	/** TSID primary key (lat_ prefix) */
	readonly id: string;

	/** Whether this was a user login or service account token request */
	readonly attemptType: LoginAttemptType;

	/** Whether the attempt succeeded or failed */
	readonly outcome: LoginOutcome;

	/** Reason for failure; null on success */
	readonly failureReason: LoginFailureReason | null;

	/** Email (user login) or client_id (service account) */
	readonly identifier: string;

	/** Principal ID if known (set on success, or if principal was found but auth failed) */
	readonly principalId: string | null;

	/** Client IP address */
	readonly ipAddress: string | null;

	/** User-Agent header value */
	readonly userAgent: string | null;

	/** When the attempt occurred */
	readonly attemptedAt: Date;
}
