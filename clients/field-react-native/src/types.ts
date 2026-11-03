// Shared interfaces: docs/field-mobile-libraries-spec.md §2 (copied exactly; field-js carries the same text).

export type AttestationLevel = "STRONGBOX" | "TEE" | "SOFTWARE" | "NONE";

export interface PublicEcJwk {
	kty: "EC";
	crv: "P-256";
	x: string;
	y: string;
}

export interface DeviceKey {
	jwk: PublicEcJwk;
	attestation: { level: AttestationLevel; chain?: string[] }; // chain: base64 DER certs, leaf first
}

export interface KeyProvider {
	getOrCreateDeviceKey(): Promise<DeviceKey>;
	/** ES256 over SHA-256(data); returns raw r‖s (64 bytes). */
	sign(data: Uint8Array): Promise<Uint8Array>;
	deleteDeviceKey(): Promise<void>;
}

export interface SecureStore {
	get(key: string, opts?: { prompt?: string }): Promise<string | null>;
	set(key: string, value: string, opts?: { requireBiometric?: boolean }): Promise<void>;
	delete(key: string): Promise<void>;
	biometricAvailable(): Promise<boolean>;
}

export type FieldErrorCode =
	| "KEY_INVALIDATED"
	| "BIOMETRIC_UNAVAILABLE"
	| "BIOMETRIC_CANCELLED"
	| "NO_SECURE_LOCK_SCREEN"
	| "INTERNAL";

export const FIELD_ERROR_CODES: readonly FieldErrorCode[] = [
	"KEY_INVALIDATED",
	"BIOMETRIC_UNAVAILABLE",
	"BIOMETRIC_CANCELLED",
	"NO_SECURE_LOCK_SCREEN",
	"INTERNAL",
];

/** Error thrown by providers and stores; `code` is one of FieldErrorCode. */
export class FieldError extends Error {
	readonly code: FieldErrorCode;
	constructor(code: FieldErrorCode, message: string) {
		super(message);
		this.name = "FieldError";
		this.code = code;
	}
}
