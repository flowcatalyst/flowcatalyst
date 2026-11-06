import { FIELD_ERROR_CODES, FieldError, type FieldErrorCode } from "./types";

import type { Spec } from "./NativeFcField";

/**
 * The native module surface (the codegen Spec: Android FcFieldModule / iOS FcField). Binary data crosses as
 * base64url strings. Native rejections carry `code`.
 */
export type FcFieldNative = Spec;

/** The key/store subset: what the WebView bridge and the key/store adapters use (also all iOS implements today). */
export type KeyStoreNative = Pick<
	Spec,
	| "getOrCreateDeviceKey"
	| "sign"
	| "deleteDeviceKey"
	| "storeGet"
	| "storeSet"
	| "storeDelete"
	| "biometricAvailable"
>;

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
		TurboModuleRegistry: { get(name: string): unknown };
	};
	const m = rn.TurboModuleRegistry.get("FcField") as FcFieldNative | null | undefined;
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
