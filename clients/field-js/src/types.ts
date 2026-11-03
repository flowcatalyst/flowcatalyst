// Interfaces shared with @flowcatalyst/field-react-native. Copied exactly from
// docs/field-mobile-libraries-spec.md section 2; do not diverge.

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
	set(
		key: string,
		value: string,
		opts?: { requireBiometric?: boolean },
	): Promise<void>;
	delete(key: string): Promise<void>;
	biometricAvailable(): Promise<boolean>;
}
