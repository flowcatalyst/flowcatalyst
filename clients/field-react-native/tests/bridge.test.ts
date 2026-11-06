import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createBridgeHost, jsStringLiteral, originOf, type BridgeMessageEvent } from "../src/bridge";
import type { KeyStoreNative } from "../src/native";
import { FieldError } from "../src/types";

const LS = String.fromCharCode(0x2028);
const PS = String.fromCharCode(0x2029);

function mockNative(over: Partial<KeyStoreNative> = {}): KeyStoreNative & { calls: string[] } {
	const calls: string[] = [];
	const n: KeyStoreNative = {
		getOrCreateDeviceKey: async () => {
			calls.push("getOrCreate");
			return { jwk: { kty: "EC", crv: "P-256", x: "x", y: "y" }, attestation: { level: "TEE" } };
		},
		sign: async (d) => {
			calls.push("sign:" + d);
			return "SIG";
		},
		deleteDeviceKey: async () => {
			calls.push("delete");
		},
		storeGet: async (k, p) => {
			calls.push(`get:${k}:${p}`);
			return "v";
		},
		storeSet: async (k, v, b) => {
			calls.push(`set:${k}:${v}:${b}`);
		},
		storeDelete: async (k) => {
			calls.push("del:" + k);
		},
		biometricAvailable: async () => true,
		...over,
	};
	return Object.assign(n, { calls });
}

function setup(opts: { native?: KeyStoreNative; passed?: BridgeMessageEvent[] } = {}) {
	const injected: string[] = [];
	const warnings: string[] = [];
	const host = createBridgeHost({
		allowedOrigins: ["https://app.example.com/"],
		native: () => opts.native ?? mockNative(),
		getTarget: () => ({ injectJavaScript: (s) => void injected.push(s) }),
		onMessage: (e) => void opts.passed?.push(e),
		warn: (m) => void warnings.push(m),
	});
	return { host, injected, warnings };
}

const ev = (data: unknown, url = "https://app.example.com/page?x=1"): BridgeMessageEvent => ({
	nativeEvent: { data: typeof data === "string" ? data : JSON.stringify(data), url },
});

/** Decode the JS literal the host injected back into the response object. */
function decode(script: string): Record<string, unknown> {
	const m = /^window\.__fcFieldBridge&&window\.__fcFieldBridge\.receive\((".*")\);true;$/s.exec(script);
	assert.ok(m, "script shape");
	return JSON.parse(JSON.parse(m[1] as string) as string);
}

describe("bridge host", () => {
	it("answers an allowed origin", async () => {
		const { host, injected } = setup();
		await host.handle(ev({ fcField: 1, id: "1", op: "key.getOrCreate", args: {} }));
		assert.equal(injected.length, 1);
		const r = decode(injected[0] as string);
		assert.equal(r["id"], "1");
		assert.equal(r["ok"], true);
		assert.deepEqual((r["result"] as { attestation: unknown }).attestation, { level: "TEE" });
	});

	it("does not answer a disallowed origin; warns without payload", async () => {
		const { host, injected, warnings } = setup();
		await host.handle(ev({ fcField: 1, id: "1", op: "store.get", args: { key: "SECRETKEY" } }, "https://evil.example/"));
		assert.equal(injected.length, 0);
		assert.equal(warnings.length, 1);
		assert.ok(!warnings[0]?.includes("SECRETKEY"));
		assert.ok(warnings[0]?.includes("https://evil.example"));
	});

	it("does not answer when the event has no url", async () => {
		const { host, injected } = setup();
		await host.handle({ nativeEvent: { data: JSON.stringify({ fcField: 1, id: "1", op: "bio.available", args: {} }) } });
		assert.equal(injected.length, 0);
	});

	it("passes non-bridge messages through to the app handler", async () => {
		const passed: BridgeMessageEvent[] = [];
		const { host, injected } = setup({ passed });
		const e1 = ev({ hello: "world" }, "https://other.example/");
		const e2 = ev("not json at all");
		const e3 = ev({ fcField: 2, id: "1", op: "x" });
		await host.handle(e1);
		await host.handle(e2);
		await host.handle(e3);
		assert.deepEqual(passed, [e1, e2, e3]);
		assert.equal(injected.length, 0);
	});

	it("unknown op -> INTERNAL", async () => {
		const { host, injected } = setup();
		await host.handle(ev({ fcField: 1, id: "9", op: "nope", args: {} }));
		const r = decode(injected[0] as string);
		assert.equal(r["ok"], false);
		assert.equal((r["error"] as { code: string }).code, "INTERNAL");
	});

	it("maps every op to the native module", async () => {
		const native = mockNative();
		const { host, injected } = setup({ native });
		const ops: [string, Record<string, unknown>][] = [
			["key.sign", { data: "AQID" }],
			["key.delete", {}],
			["store.get", { key: "k", prompt: "P" }],
			["store.set", { key: "k", value: "v", requireBiometric: true }],
			["store.delete", { key: "k" }],
			["bio.available", {}],
		];
		for (const [i, [op, args]] of ops.entries()) await host.handle(ev({ fcField: 1, id: String(i), op, args }));
		assert.deepEqual(native.calls, ["sign:AQID", "delete", "get:k:P", "set:k:v:true", "del:k"]);
		const results = injected.map((s) => decode(s)["result"]);
		assert.deepEqual(results, [{ signature: "SIG" }, {}, { value: "v" }, {}, {}, { available: true }]);
	});

	it("passes native error codes through, unknown codes become INTERNAL", async () => {
		const native = mockNative({
			sign: async () => {
				throw Object.assign(new Error("gone"), { code: "KEY_INVALIDATED" });
			},
			storeGet: async () => {
				throw new FieldError("BIOMETRIC_CANCELLED", "cancelled");
			},
			storeDelete: async () => {
				throw Object.assign(new Error("weird"), { code: "E_WHATEVER" });
			},
		});
		const { host, injected } = setup({ native });
		await host.handle(ev({ fcField: 1, id: "a", op: "key.sign", args: { data: "AQ" } }));
		await host.handle(ev({ fcField: 1, id: "b", op: "store.get", args: { key: "k" } }));
		await host.handle(ev({ fcField: 1, id: "c", op: "store.delete", args: { key: "k" } }));
		const codes = injected.map((s) => (decode(s)["error"] as { code: string }).code);
		assert.deepEqual(codes, ["KEY_INVALIDATED", "BIOMETRIC_CANCELLED", "INTERNAL"]);
	});

	it("escapes quotes, </script>, U+2028/2029 in injected scripts", async () => {
		const nasty = `a"b'c</script><!--${LS}${PS}\\end`;
		const native = mockNative({ storeGet: async () => nasty });
		const { host, injected } = setup({ native });
		await host.handle(ev({ fcField: 1, id: "q", op: "store.get", args: { key: "k" } }));
		const script = injected[0] as string;
		assert.ok(!script.includes("</script>"));
		assert.ok(!script.includes(LS) && !script.includes(PS));
		assert.ok(!script.includes("<"));
		// the script must parse as JS and round-trip the value
		const received: string[] = [];
		new Function("window", script)({ __fcFieldBridge: { receive: (m: string) => void received.push(m) } });
		const r = JSON.parse(received[0] as string);
		assert.equal(r.result.value, nasty);
	});

	it("never logs values, data or signatures", async () => {
		const logged: string[] = [];
		const orig = { log: console.log, warn: console.warn, error: console.error, info: console.info };
		for (const k of ["log", "warn", "error", "info"] as const) console[k] = (...a: unknown[]) => void logged.push(a.join(" "));
		try {
			const host = createBridgeHost({
				allowedOrigins: ["https://app.example.com"],
				native: () => mockNative({ sign: async () => "SIGSECRET", storeGet: async () => { throw new Error("boom"); } }),
				getTarget: () => ({ injectJavaScript: () => {} }),
			});
			await host.handle(ev({ fcField: 1, id: "1", op: "key.sign", args: { data: "DATASECRET" } }));
			await host.handle(ev({ fcField: 1, id: "2", op: "store.set", args: { key: "k", value: "VALSECRET" } }));
			await host.handle(ev({ fcField: 1, id: "3", op: "store.get", args: { key: "k" } }, "https://evil.example"));
			host.onMessage(ev("{bad"));
		} finally {
			Object.assign(console, orig);
		}
		for (const l of logged) assert.ok(!/SECRET/.test(l), l);
	});

	it("origin helpers", () => {
		assert.equal(originOf("HTTPS://App.Example.com:8443/a/b?c#d"), "https://app.example.com:8443");
		assert.equal(originOf("about:blank"), null);
		assert.equal(jsStringLiteral(LS + "<"), '"\\u2028\\u003c"');
	});
});
