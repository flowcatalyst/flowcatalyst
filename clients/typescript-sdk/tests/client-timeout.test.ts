import { test, mock } from "node:test";
import assert from "node:assert/strict";
import { FlowCatalystClient } from "../src/index.js";

// Regression: the request timeout used to create an AbortController and a
// timer but never hand the signal to the request, so a hung server was never
// aborted and `timeout` had no effect.

function hangingFetch(seen: { signal?: AbortSignal | null }) {
	return (input: Request | string | URL, init?: RequestInit) => {
		void input;
		const signal = init?.signal;
		seen.signal = signal;
		return new Promise<Response>((_resolve, reject) => {
			signal?.addEventListener("abort", () => {
				const e = new Error("The operation was aborted");
				e.name = "AbortError";
				reject(e);
			});
		});
	};
}

test("a request that outlives `timeout` is aborted and surfaces a timeout error", async () => {
	const realFetch = globalThis.fetch;
	const seen: { signal?: AbortSignal | null } = {};
	globalThis.fetch = hangingFetch(seen) as typeof fetch;
	mock.timers.enable({ apis: ["setTimeout"] });
	try {
		const client = new FlowCatalystClient({
			baseUrl: "http://127.0.0.1:1",
			accessToken: "t",
			timeout: 5_000,
			retryAttempts: 0,
		});
		const pending = client.eventTypes().list();
		// Let the request reach fetch (token provider + request build are async).
		for (let i = 0; i < 20 && !seen.signal; i++) {
			await new Promise<void>((r) => setImmediate(r));
		}
		assert.ok(seen.signal, "fetch must receive an abort signal");
		assert.equal(seen.signal.aborted, false, "not aborted before the timeout");
		mock.timers.tick(5_000);
		const result = await pending;
		assert.ok(result.isErr(), "expected an error result");
		if (!result.isErr()) throw new Error("unreachable");
		assert.equal(result.error.type, "timeout");
		assert.equal(seen.signal.aborted, true);
	} finally {
		mock.timers.reset();
		globalThis.fetch = realFetch;
	}
});

test("a fast response is unaffected and the timer is cleared", async () => {
	const realFetch = globalThis.fetch;
	globalThis.fetch = (async () =>
		new Response(JSON.stringify({ eventTypes: [] }), {
			status: 200,
			headers: { "content-type": "application/json" },
		})) as typeof fetch;
	try {
		const client = new FlowCatalystClient({
			baseUrl: "http://127.0.0.1:1",
			accessToken: "t",
			timeout: 5_000,
			retryAttempts: 0,
		});
		const result = await client.eventTypes().list();
		assert.ok(result.isOk(), JSON.stringify(result));
	} finally {
		globalThis.fetch = realFetch;
	}
});
