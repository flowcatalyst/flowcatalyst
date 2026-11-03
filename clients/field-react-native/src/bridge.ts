// WebView bridge host, protocol version 1: docs/field-mobile-libraries-spec.md §3.
import { fromBase64Url, toBase64Url } from "./base64url";
import { toFieldError, type FcFieldNative } from "./native";
import { FieldError } from "./types";

export interface BridgeMessageEvent {
	nativeEvent: { data: string; url?: string };
}

export interface InjectTarget {
	injectJavaScript(script: string): void;
}

export interface BridgeHostOptions {
	allowedOrigins: string[];
	native: () => FcFieldNative;
	getTarget: () => InjectTarget | null | undefined;
	/** The app's own onMessage handler; receives every message that is not a bridge request. */
	onMessage?: (event: BridgeMessageEvent) => void;
	warn?: (msg: string) => void;
}

/** Installed before content loads; makes bridgeAvailable() true on the page. */
export const INJECTED_BEFORE_CONTENT_LOADED =
	"(function(){if(window.__fcFieldBridge)return;window.__fcFieldBridge={version:1,receive:function(m){" +
	"var l=window.__fcFieldBridge._l;if(l)l(m);else(window.__fcFieldBridge._q=window.__fcFieldBridge._q||[]).push(m);}};})();true;";

export function originOf(url: string | undefined): string | null {
	if (!url) return null;
	const m = /^([a-z][a-z0-9+.-]*:\/\/[^/?#]*)/i.exec(url);
	return m && m[1] ? m[1].toLowerCase() : null;
}

function normaliseOrigin(o: string): string {
	return (originOf(o) ?? o).toLowerCase();
}

/** Escape a JSON string so it is safe inside a JS string literal passed to injectJavaScript. */
export function jsStringLiteral(s: string): string {
	return JSON.stringify(s)
		.split(String.fromCharCode(0x2028)).join("\\u2028")
		.split(String.fromCharCode(0x2029)).join("\\u2029")
		.replace(/</g, "\\u003c");
}

interface BridgeRequest {
	fcField: 1;
	id: string;
	op: string;
	args: Record<string, unknown>;
}

function parseRequest(data: string): BridgeRequest | null {
	let v: unknown;
	try {
		v = JSON.parse(data);
	} catch {
		return null;
	}
	if (typeof v !== "object" || v === null) return null;
	const o = v as Record<string, unknown>;
	if (o["fcField"] !== 1) return null;
	if (typeof o["id"] !== "string" || typeof o["op"] !== "string") return null;
	const args = o["args"];
	return {
		fcField: 1,
		id: o["id"],
		op: o["op"],
		args: typeof args === "object" && args !== null ? (args as Record<string, unknown>) : {},
	};
}

function str(args: Record<string, unknown>, k: string): string {
	const v = args[k];
	if (typeof v !== "string") throw new FieldError("INTERNAL", `bad argument: ${k}`);
	return v;
}

async function dispatch(native: FcFieldNative, req: BridgeRequest): Promise<unknown> {
	const a = req.args;
	switch (req.op) {
		case "key.getOrCreate":
			return await native.getOrCreateDeviceKey();
		case "key.sign": {
			const data = str(a, "data");
			fromBase64Url(data); // validates; native receives the base64url as-is
			return { signature: await native.sign(data) };
		}
		case "key.delete":
			await native.deleteDeviceKey();
			return {};
		case "store.get": {
			const prompt = typeof a["prompt"] === "string" ? a["prompt"] : null;
			return { value: await native.storeGet(str(a, "key"), prompt) };
		}
		case "store.set":
			await native.storeSet(str(a, "key"), str(a, "value"), a["requireBiometric"] === true);
			return {};
		case "store.delete":
			await native.storeDelete(str(a, "key"));
			return {};
		case "bio.available":
			return { available: await native.biometricAvailable() };
		default:
			throw new FieldError("INTERNAL", `unknown op: ${req.op}`);
	}
}

export interface BridgeHost {
	/** Pass as <WebView onMessage>. */
	onMessage(event: BridgeMessageEvent): void;
	/** Resolves when any in-flight response for this message has been injected (for tests). */
	handle(event: BridgeMessageEvent): Promise<void>;
}

export function createBridgeHost(opts: BridgeHostOptions): BridgeHost {
	const allowed = new Set(opts.allowedOrigins.map(normaliseOrigin));
	const warn = opts.warn ?? ((m: string) => console.warn(m));

	async function handle(event: BridgeMessageEvent): Promise<void> {
		const data = event.nativeEvent.data;
		const req = typeof data === "string" ? parseRequest(data) : null;
		if (!req) {
			opts.onMessage?.(event);
			return;
		}
		const origin = originOf(event.nativeEvent.url);
		if (!origin || !allowed.has(origin)) {
			// No answer. Log the origin only, never the payload.
			warn(`[field-bridge] ignored request from origin not in allowedOrigins: ${origin ?? "unknown"}`);
			return;
		}
		let response: Record<string, unknown>;
		try {
			const result = await dispatch(opts.native(), req);
			response = { fcField: 1, id: req.id, ok: true, result };
		} catch (e) {
			const fe = toFieldError(e);
			response = { fcField: 1, id: req.id, ok: false, error: { code: fe.code, message: fe.message } };
		}
		const target = opts.getTarget();
		if (!target) return;
		target.injectJavaScript(
			`window.__fcFieldBridge&&window.__fcFieldBridge.receive(${jsStringLiteral(JSON.stringify(response))});true;`,
		);
	}

	return {
		handle,
		onMessage(event) {
			void handle(event).catch(() => {
				warn("[field-bridge] internal error handling message");
			});
		},
	};
}

export { toBase64Url };
