import assert from "node:assert/strict";
import { afterEach, beforeEach, describe, it } from "node:test";
import { PushError, PushStream, type PushMessage, type PushStreamState } from "../src/index";

const BASE = "https://app.test/api";
const enc = new TextEncoder();

function sse(id: string | null, type: string, data: unknown): string {
	return `${id === null ? "" : `id: ${id}\n`}event: ${type}\ndata: ${JSON.stringify(data)}\n\n`;
}

/** The whole stream as bytes, cut at the given offsets (which may fall inside a multi-byte character). */
function chunked(text: string, cuts: number[]): Uint8Array[] {
	const all = enc.encode(text);
	const out: Uint8Array[] = [];
	let at = 0;
	for (const c of [...cuts, all.length]) {
		out.push(all.slice(at, c));
		at = c;
	}
	return out;
}

/** A body that serves `chunks`, then stays open until the request is aborted. */
// (A stub, not a Response: Node's Response loads undici, which needs the TextDecoder these tests remove.)
function holdOpen(chunks: Uint8Array[], signal: AbortSignal): Response {
	const body = new ReadableStream<Uint8Array>({
		start(c) {
			for (const ch of chunks) c.enqueue(ch);
			signal.addEventListener("abort", () => c.error(new Error("aborted")), { once: true });
		},
	});
	return { ok: true, status: 200, body } as unknown as Response;
}

async function until(cond: () => boolean, what: string): Promise<void> {
	for (let i = 0; i < 200; i++) {
		if (cond()) return;
		await new Promise((r) => setTimeout(r, 5));
	}
	assert.fail(`timed out waiting for ${what}`);
}

interface Rig {
	stream: PushStream;
	pushes: PushMessage[];
	events: string[];
	subscribes: unknown[];
	gets: { url: URL; headers: Record<string, string> }[];
}

function rig(streamText: string, cuts: number[]): Rig {
	const subscribes: unknown[] = [];
	const gets: Rig["gets"] = [];
	const subscribeFetch = (async (_url: string, init: RequestInit) => {
		subscribes.push(JSON.parse(String(init.body)));
		return { ok: true, status: 200, json: async () => ({ streamToken: "tok", granted: ["a:b"], denied: [] }) };
	}) as unknown as typeof fetch;
	const streamFetch = (async (url: string, init: RequestInit) => {
		gets.push({ url: new URL(url), headers: init.headers as Record<string, string> });
		return holdOpen(chunked(streamText, cuts), init.signal as AbortSignal);
	}) as unknown as typeof fetch;
	const stream = new PushStream({
		baseUrl: BASE,
		topics: ["a:b"],
		fetch: subscribeFetch,
		streamFetch,
		sleep: () => new Promise(() => {}),
	});
	const pushes: PushMessage[] = [];
	const events: string[] = [];
	stream.onAny((p) => pushes.push(p));
	stream.addListener("subscribed", (d) => events.push(`subscribed:${d.granted.join(",")}:${d.latest}`));
	stream.addListener("reset", (d) => events.push(`reset:${d.reason}:${d.latest}`));
	return { stream, pushes, events, subscribes, gets };
}

describe("PushStream (React Native)", () => {
	const saved = globalThis.TextDecoder;
	beforeEach(() => {
		delete (globalThis as { TextDecoder?: unknown }).TextDecoder;
	});
	afterEach(() => {
		globalThis.TextDecoder = saved;
	});

	it("reads subscribed, ordered pushes and a reset with no TextDecoder, cut mid-line and mid-character", async () => {
		const text =
			sse(null, "subscribed", { granted: ["a:b"], latest: 4 }) +
			sse("5", "job.changed", { topic: "a:b", data: { note: "naïve €5 😀" } }) +
			sse("6", "job.changed", { topic: "a:b", data: { n: 2 } }) +
			sse("10", "reset", { reason: "retention", latest: 10 });
		const bytes = enc.encode(text);
		// cuts inside the first line, inside "ï" (2 bytes), inside "€" and inside the emoji
		const at = (s: string) => {
			const needle = enc.encode(s);
			outer: for (let i = 0; i < bytes.length; i++) {
				for (let j = 0; j < needle.length; j++) if (bytes[i + j] !== needle[j]) continue outer;
				return i;
			}
			throw new Error("not found");
		};
		const cuts = [7, at("ï") + 1, at("€") + 2, at("😀") + 1, at("😀") + 3, at('"n"') + 2];
		const r = rig(text, cuts);
		const states: PushStreamState[] = [];
		r.stream.addListener("state", (s) => states.push(s));
		assert.equal(globalThis.TextDecoder, undefined);
		r.stream.start();
		await until(() => r.events.length >= 2, "reset");
		assert.deepEqual(r.subscribes, [{ topics: ["a:b"], tickets: [] }]);
		assert.deepEqual(r.events, ["subscribed:a:b:4", "reset:retention:10"]);
		assert.deepEqual(
			r.pushes.map((p) => [p.id, p.topic, p.type, p.data]),
			[
				["5", "a:b", "job.changed", { note: "naïve €5 😀" }],
				["6", "a:b", "job.changed", { n: 2 }],
			],
		);
		assert.equal(r.stream.lastEventId, "10");
		assert.equal(r.stream.state, "open");
		r.stream.stop();
		assert.equal(r.stream.state, "stopped");
		assert.deepEqual(states, ["connecting", "open", "stopped"]);
		assert.equal(r.gets.length, 1);
		assert.equal(r.gets[0]?.url.searchParams.get("st"), "tok");
	});

	it("addListener(...).remove() stops delivery to that listener only", async () => {
		const text =
			sse(null, "subscribed", { granted: ["a:b"], latest: 0 }) + sse("1", "reset", { reason: "unknown-id", latest: 1 });
		const r = rig(text, []);
		const a: string[] = [];
		const b: string[] = [];
		const ha = r.stream.addListener("reset", (d) => a.push(d.reason));
		r.stream.addListener("reset", (d) => b.push(d.reason));
		ha.remove();
		r.stream.start();
		await until(() => b.length === 1, "reset");
		r.stream.stop();
		assert.deepEqual(a, []);
		assert.deepEqual(b, ["unknown-id"]);
	});

	it("a throwing listener does not stop the others, and the on() remover works", async () => {
		const text =
			sse(null, "subscribed", { granted: ["a:b"], latest: 0 }) + sse("1", "j", { topic: "a:b", data: 1 }) + sse("2", "j", { topic: "a:b", data: 2 });
		const r = rig(text, []);
		const seen: number[] = [];
		r.stream.addListener("subscribed", () => {
			throw new Error("boom");
		});
		const off = r.stream.on("a:*", (p) => {
			seen.push(Number(p.id));
			off();
		});
		r.stream.start();
		await until(() => r.pushes.length === 2 && r.events.length === 1, "pushes");
		r.stream.stop();
		assert.deepEqual(seen, [1]);
	});
});

describe("PushStream (React Native) errors", () => {
	it("reports a failed subscribe as a PushError on the error listener", async () => {
		const errors: PushError[] = [];
		const stream = new PushStream({
			baseUrl: BASE,
			topics: ["a:b"],
			fetch: (async () => new Response(JSON.stringify({ error: "NO_TOPICS", message: "none" }), { status: 403 })) as unknown as typeof fetch,
		});
		stream.addListener("error", (e) => errors.push(e));
		stream.start();
		await until(() => errors.length > 0 && stream.state === "stopped", "stop");
		assert.ok(errors[0] instanceof PushError);
		assert.equal(errors[0]?.code, "NO_TOPICS");
	});
});
