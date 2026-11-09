/** Codes carried by errors from key providers and secure stores (spec section 2). */
export type ProviderErrorCode =
	| "KEY_INVALIDATED"
	| "BIOMETRIC_UNAVAILABLE"
	| "BIOMETRIC_CANCELLED"
	| "NO_SECURE_LOCK_SCREEN"
	| "INTERNAL";

export const PROVIDER_ERROR_CODES: readonly ProviderErrorCode[] = [
	"KEY_INVALIDATED",
	"BIOMETRIC_UNAVAILABLE",
	"BIOMETRIC_CANCELLED",
	"NO_SECURE_LOCK_SCREEN",
	"INTERNAL",
];

export function isProviderErrorCode(v: unknown): v is ProviderErrorCode {
	return (
		typeof v === "string" &&
		(PROVIDER_ERROR_CODES as readonly string[]).includes(v)
	);
}

/** Thrown by key providers and secure stores. */
export class FieldProviderError extends Error {
	readonly code: ProviderErrorCode;
	constructor(code: ProviderErrorCode, message?: string) {
		super(message ?? code);
		this.name = "FieldProviderError";
		this.code = code;
	}
}

/**
 * The platform's error codes (docs/wire-contract.md "Field identity"), plus the
 * library's own. The platform also sends `invalid_dpop_proof`,
 * `use_dpop_nonce` and `invalid_token` in lower case; they pass through as is.
 */
export type FieldAuthErrorCode =
	| "INVALID_CREDENTIALS"
	| "WORKER_LOCKED"
	| "WORKER_LOCKED_TEMPORARILY"
	| "TOO_MANY_ATTEMPTS"
	| "NO_APP_ACCESS"
	| "DEVICE_REVOKED"
	| "DEVICE_SUSPENDED"
	| "UNKNOWN_DEVICE"
	| "INVALID_GRANT"
	| "SESSION_EXPIRED"
	| "PIN_POLICY"
	| "INVALID_ENROLMENT_CODE"
	| "FIELD_IDENTITY_UNCONFIGURED"
	| "NETWORK"
	| "NOT_ENROLLED"
	| "BIOMETRIC_INVALIDATED"
	| "CLIENT_MISMATCH"
	| "PIN_REQUIRED"
	| "IMPERSONATING"
	| "IMPERSONATION_FORBIDDEN"
	| "INVALID_CODE"
	| "INVALID_REQUEST"
	| "NOT_IMPERSONATING"
	| ProviderErrorCode
	| (string & {});

/**
 * An error from the library. Its message never contains a PIN or a refresh
 * token.
 */
export class FieldAuthError extends Error {
	readonly code: FieldAuthErrorCode;
	/** HTTP status when the platform answered. */
	readonly status?: number;
	/** Seconds, from `Retry-After` (TOO_MANY_ATTEMPTS, WORKER_LOCKED_TEMPORARILY). */
	readonly retryAfter?: number;
	/** PIN_POLICY violation codes. */
	readonly violations?: string[];

	constructor(
		code: FieldAuthErrorCode,
		message?: string,
		extra: { status?: number; retryAfter?: number; violations?: string[] } = {},
	) {
		super(message ?? code);
		this.name = "FieldAuthError";
		this.code = code;
		if (extra.status !== undefined) this.status = extra.status;
		if (extra.retryAfter !== undefined) this.retryAfter = extra.retryAfter;
		if (extra.violations !== undefined) this.violations = extra.violations;
	}
}

// The push reader's error lives in push-core.ts (which imports nothing, so field-react-native can carry a verbatim copy).
export { PushError } from "./push-core.js";
export type { PushErrorCode } from "./push-core.js";
