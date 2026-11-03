export {
	FieldAuth,
	FieldAuthEvent,
	parseEnrolmentQr,
	REFRESH_SKEW_MS,
	STORE_DEVICE,
	STORE_REFRESH,
	STORE_SESSION,
} from "./auth.js";
export type {
	DeviceInfo,
	DeviceRecord,
	EnrolOptions,
	FieldAuthEventType,
	FieldAuthOptions,
	LockedDetail,
	QrPayload,
	Session,
	SessionEndedReason,
} from "./auth.js";
export {
	BRIDGE_TIMEOUT_MS,
	BridgeClient,
	BridgeKeyProvider,
	BridgeSecureStore,
	bridgeAvailable,
	defaultProviders,
} from "./bridge.js";
export type { BridgeClientOptions, BridgeGlobal } from "./bridge.js";
export { FieldAuthError, FieldProviderError } from "./errors.js";
export type { FieldAuthErrorCode, ProviderErrorCode } from "./errors.js";
export { definePinPad } from "./pin-pad.js";
export type { FcPinPadElement, PinEventDetail } from "./pin-pad.js";
export { accessTokenHash, buildProof, jwkThumbprint } from "./proof.js";
export {
	IdbDb,
	IndexedDbSecureStore,
	MemoryDb,
	MemorySecureStore,
	WebCryptoKeyProvider,
} from "./storage.js";
export type { KeyValueDb } from "./storage.js";
export type {
	AttestationLevel,
	DeviceKey,
	KeyProvider,
	PublicEcJwk,
	SecureStore,
} from "./types.js";

import { definePinPad } from "./pin-pad.js";

// Register <fc-pin-pad> when there is a DOM.
definePinPad();
