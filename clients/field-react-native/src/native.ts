import { FIELD_ERROR_CODES, FieldError, type DeviceKey, type FieldErrorCode } from "./types";

/**
 * The native module surface (Android FcFieldModule / iOS FcField). Binary data crosses as
 * base64url strings. Native rejections carry `code` (one of FieldErrorCode).
 */
export interface FcFieldNative {
	getOrCreateDeviceKey(): Promise<DeviceKey>;
	/** dataB64u -> base64url(raw r‖s) */
	sign(dataB64u: string): Promise<string>;
	deleteDeviceKey(): Promise<void>;
	storeGet(key: string, prompt: string | null): Promise<string | null>;
	storeSet(key: string, value: string, requireBiometric: boolean): Promise<void>;
	storeDelete(key: string): Promise<void>;
	biometricAvailable(): Promise<boolean>;
}

let override: FcFieldNative | undefined;

/** Test hook / manual wiring. */
export function setNativeModule(m: FcFieldNative | undefined): void {
	override = m;
}

export function getNativeModule(): FcFieldNative {
	if (override) return override;
	// Lazy require so this file can be loaded outside React Native (tests, tooling).
	// eslint-disable-next-line @typescript-eslint/no-require-imports
	const rn = require("react-native") as {
		NativeModules: Record<string, unknown>;
	};
	const m = rn.NativeModules["FcField"] as FcFieldNative | undefined;
	if (!m) {
		throw new FieldError(
			"INTERNAL",
			"FcField native module is not linked (rebuild the app after installing @flowcatalyst/field-react-native)",
		);
	}
	return m;
}

declare const require: (id: string) => unknown;

/** Normalise a native rejection into a FieldError. Never includes payloads. */
export function toFieldError(e: unknown): FieldError {
	if (e instanceof FieldError) return e;
	const err = e as { code?: unknown; message?: unknown } | null;
	const code = typeof err?.code === "string" ? err.code : "";
	const known = (FIELD_ERROR_CODES as readonly string[]).includes(code) ? (code as FieldErrorCode) : "INTERNAL";
	const message = typeof err?.message === "string" ? err.message : "native error";
	return new FieldError(known, message);
}
