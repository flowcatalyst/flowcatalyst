import { fromBase64Url, toBase64Url } from "./base64url";
import { getNativeModule, toFieldError, type FcFieldNative } from "./native";
import type { DeviceKey, KeyProvider } from "./types";

export class NativeKeyProvider implements KeyProvider {
	private readonly native: () => FcFieldNative;

	constructor(native?: FcFieldNative) {
		this.native = native ? () => native : getNativeModule;
	}

	async getOrCreateDeviceKey(): Promise<DeviceKey> {
		try {
			return await this.native().getOrCreateDeviceKey();
		} catch (e) {
			throw toFieldError(e);
		}
	}

	async sign(data: Uint8Array): Promise<Uint8Array> {
		try {
			return fromBase64Url(await this.native().sign(toBase64Url(data)));
		} catch (e) {
			throw toFieldError(e);
		}
	}

	async deleteDeviceKey(): Promise<void> {
		try {
			await this.native().deleteDeviceKey();
		} catch (e) {
			throw toFieldError(e);
		}
	}
}
