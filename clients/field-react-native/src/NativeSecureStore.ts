import { getNativeModule, toFieldError, type KeyStoreNative } from "./native";
import type { SecureStore } from "./types";

export class NativeSecureStore implements SecureStore {
	private readonly native: () => KeyStoreNative;

	constructor(native?: KeyStoreNative) {
		this.native = native ? () => native : getNativeModule;
	}

	async get(key: string, opts?: { prompt?: string }): Promise<string | null> {
		try {
			return await this.native().storeGet(key, opts?.prompt ?? null);
		} catch (e) {
			throw toFieldError(e);
		}
	}

	async set(key: string, value: string, opts?: { requireBiometric?: boolean }): Promise<void> {
		try {
			await this.native().storeSet(key, value, opts?.requireBiometric === true);
		} catch (e) {
			throw toFieldError(e);
		}
	}

	async delete(key: string): Promise<void> {
		try {
			await this.native().storeDelete(key);
		} catch (e) {
			throw toFieldError(e);
		}
	}

	async biometricAvailable(): Promise<boolean> {
		try {
			return await this.native().biometricAvailable();
		} catch (e) {
			throw toFieldError(e);
		}
	}
}
