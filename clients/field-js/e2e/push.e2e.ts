// End-to-end of PushStream against the framework's push demo server
// (flowcatalyst-framework go/examples/push-demo). Skips unless FIELD_PUSH_E2E_URL is set.
//
//   FIELD_PUSH_E2E_URL            the demo's API base, e.g. http://127.0.0.1:18300/api (required)
//   FIELD_PUSH_E2E_RESTART_CMD    a shell command that kills the demo hard and starts it again (restart scenario)
//   FIELD_PUSH_E2E_SHORT_URL      a second demo started with PUSH_STREAM_MAX_LIFETIME=1m (auth-expired scenario)
//
// The demo grants any `pushdemo:t:{id}` topic to a ticket from GET {base}/demo/ticket and publishes
// through POST {base}/demo/publish. Topics are unique per test, so tests do not see each other's pushes.

import { strict as assert } from "node:assert";
import { spawnSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import { describe, it } from "node:test";
import { PushError, PushStream, PushStreamEvent, type PushMessage, type PushStreamState } from "../src/index.js";

const base = process.env["FIELD_PUSH_E2E_URL"];
const restartCmd = process.env["FIELD_PUSH_E2E_RESTART_CMD"];
const shortBase = process.env["FIELD_PUSH_E2E_SHORT_URL"];

const topic = (tag: string) => `pushdemo:t:${tag}-${randomUUID().slice(0, 8)}`;

async function tickets(url: string): Promise<string[]> {
	const res = await fetch(`${url}/demo/ticket`);
	assert.equal(res.status, 200);
	return [((await res.json()) as { ticket: string }).ticket];
}

async function publish(url: string, items: { seq: number; topic: string }[]): Promise<void> {
	const res = await fetch(`${url}/demo/publish`, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ items }),
	});
	assert.equal(res.status, 200);
	const a = (await res.json()) as { published: number; failed: unknown[] | null };
	assert.equal(a.published, items.length, `publish failed: ${JSON.stringify(a)}`);
}

/** One push at a time, so the log order is the call order. */
async function publishSeq(url: string, t: string, from: number, count: number): Promise<void> {
	for (let i = from; i < from + count; i++) await publish(url, [{ seq: i, topic: t }]);
}

function sleep(ms: number): Promise<void> {
	return new Promise((r) => setTimeout(r, ms));
}

async function until(what: string, fn: () => boolean, ms = 30_000): Promise<void> {
	const end = Date.now() + ms;
	while (!fn()) {
		if (Date.now() > end) assert.fail(`timed out waiting for ${what}`);
		await sleep(25);
	}
}

interface Probe {
	stream: PushStream;
	states: PushStreamState[];
	subscribed: { granted: string[]; latest: string }[];
	denied: { topic: string; code: string }[][];
	resets: { reason: string; latest: string }[];
	errors: PushError[];
}

function probe(url: string, topics: string[], extra: { lastEventId?: string; getTickets?: boolean } = {}): Probe {
	const stream = new PushStream({
		baseUrl: url,
		topics,
		fetch: (i, init) => fetch(i, init),
		streamFetch: (i, init) => fetch(i, init),
		...(extra.getTickets === false ? {} : { getTickets: () => tickets(url) }),
		...(extra.lastEventId ? { lastEventId: extra.lastEventId } : {}),
	});
	const p: Probe = { stream, states: [], subscribed: [], denied: [], resets: [], errors: [] };
	const det = <T>(e: Event) => (e as PushStreamEvent<T>).detail;
	stream.addEventListener("state", (e) => p.states.push(det<PushStreamState>(e)));
	stream.addEventListener("subscribed", (e) => p.subscribed.push(det(e)));
	stream.addEventListener("denied", (e) => p.denied.push(det(e)));
	stream.addEventListener("reset", (e) => p.resets.push(det(e)));
	stream.addEventListener("error", (e) => p.errors.push(det<PushError>(e)));
	return p;
}

const seqs = (ms: PushMessage[]) => ms.map((m) => (m.data as { seq: number }).seq);
const range = (from: number, n: number) => Array.from({ length: n }, (_, i) => from + i);
const ascending = (ms: PushMessage[]) => ms.every((m, i) => i === 0 || BigInt(m.id) > BigInt(ms[i - 1]!.id));

describe("PushStream against the push demo", { skip: base ? false : "FIELD_PUSH_E2E_URL is not set" }, () => {
	const url = base ?? "";

	it("subscribes, reports the granted topics and delivers a push once", async () => {
		const t = topic("basic");
		const p = probe(url, [t]);
		const got: PushMessage[] = [];
		p.stream.on(t, (m) => got.push(m));
		p.stream.start();
		try {
			await until("subscribed", () => p.subscribed.length > 0);
			assert.deepEqual(p.subscribed[0]!.granted, [t]);
			assert.match(p.subscribed[0]!.latest, /^\d+$/);
			assert.equal(p.stream.state, "open");
			await publish(url, [{ seq: 7, topic: t }]);
			await until("the push", () => got.length > 0);
			await sleep(300);
			assert.equal(got.length, 1);
			const m = got[0]!;
			assert.match(m.id, /^\d+$/);
			assert.equal(m.topic, t);
			assert.equal(m.type, "demo.tick");
			assert.equal((m.data as { seq: number }).seq, 7);
			assert.equal(p.stream.lastEventId, m.id);
			assert.deepEqual(p.errors, []);
		} finally {
			p.stream.stop();
		}
	});

	it("delivers to pattern handlers and onAny", async () => {
		const id = randomUUID().slice(0, 8);
		const a = `pushdemo:t:${id}-a`;
		const b = `pushdemo:t:${id}-b`;
		const p = probe(url, [a, b]);
		const star: string[] = [];
		const deep: string[] = [];
		const any: string[] = [];
		const exact: string[] = [];
		const miss: string[] = [];
		p.stream.on("pushdemo:t:*", (m) => star.push(m.topic));
		p.stream.on("pushdemo:**", (m) => deep.push(m.topic));
		p.stream.on("pushdemo:*", (m) => miss.push(m.topic)); // one segment short of the topics
		p.stream.on(a, (m) => exact.push(m.topic));
		p.stream.onAny((m) => any.push(m.topic));
		p.stream.start();
		try {
			await until("subscribed", () => p.subscribed.length > 0);
			assert.deepEqual([...p.subscribed[0]!.granted].sort(), [a, b].sort());
			await publish(url, [{ seq: 1, topic: a }]);
			await publish(url, [{ seq: 2, topic: b }]);
			await until("both pushes", () => any.length >= 2);
			await sleep(200);
			assert.deepEqual(star, [a, b]);
			assert.deepEqual(deep, [a, b]);
			assert.deepEqual(any, [a, b]);
			assert.deepEqual(exact, [a]);
			assert.deepEqual(miss, []);
		} finally {
			p.stream.stop();
		}
	});

	it("keeps order and drops nothing over a burst of 200", async () => {
		const t = topic("burst");
		const p = probe(url, [t]);
		const got: PushMessage[] = [];
		p.stream.onAny((m) => got.push(m));
		p.stream.start();
		try {
			await until("subscribed", () => p.subscribed.length > 0);
			await publishSeq(url, t, 0, 200);
			await until("200 pushes", () => got.length >= 200);
			await sleep(300);
			assert.equal(got.length, 200);
			assert.deepEqual(seqs(got), range(0, 200));
			assert.ok(ascending(got));
			assert.equal(new Set(got.map((m) => m.id)).size, 200);
		} finally {
			p.stream.stop();
		}
	});

	it("resumes from lastEventId: exactly the pushes published while it was stopped, in order", async () => {
		const t = topic("resume");
		const first = probe(url, [t]);
		const got1: PushMessage[] = [];
		first.stream.onAny((m) => got1.push(m));
		first.stream.start();
		await until("subscribed", () => first.subscribed.length > 0);
		await publishSeq(url, t, 0, 5);
		await until("5 pushes", () => got1.length >= 5);
		first.stream.stop();
		const last = first.stream.lastEventId;
		assert.ok(last);
		assert.equal(first.stream.state, "stopped");

		await publishSeq(url, t, 5, 25);

		// A new stream object (as after an app restart), given the persisted id.
		const second = probe(url, [t], { lastEventId: last });
		const got2: PushMessage[] = [];
		second.stream.onAny((m) => got2.push(m));
		second.stream.start();
		try {
			await until("25 pushes", () => got2.length >= 25);
			await sleep(300);
			assert.deepEqual(seqs(got2), range(5, 25));
			assert.ok(ascending(got2));
			assert.deepEqual(second.resets, []);
		} finally {
			second.stream.stop();
		}

		// And stop/start on the same object keeps its own last id.
		const third = probe(url, [t]);
		const got3: PushMessage[] = [];
		third.stream.onAny((m) => got3.push(m));
		third.stream.start();
		await until("subscribed", () => third.subscribed.length > 0);
		await publishSeq(url, t, 30, 2);
		await until("2 pushes", () => got3.length >= 2);
		third.stream.stop();
		await publishSeq(url, t, 32, 3);
		third.stream.start();
		try {
			await until("3 more", () => got3.length >= 5);
			await sleep(300);
			assert.deepEqual(seqs(got3), range(30, 5));
		} finally {
			third.stream.stop();
		}
	});

	it("answers a Last-Event-ID the log does not know with a reset", async () => {
		const t = topic("unknown");
		const p = probe(url, [t], { lastEventId: "999999999999999999" });
		const got: PushMessage[] = [];
		p.stream.onAny((m) => got.push(m));
		p.stream.start();
		try {
			await until("reset", () => p.resets.length > 0);
			assert.equal(p.resets[0]!.reason, "unknown-id");
			assert.match(p.resets[0]!.latest, /^\d+$/);
			assert.ok(BigInt(p.resets[0]!.latest) < 999999999999999999n);
			assert.equal(p.stream.lastEventId, p.resets[0]!.latest);
			// the reader carries on from the reset id: a new push arrives
			await until("subscribed", () => p.subscribed.length > 0);
			await publish(url, [{ seq: 1, topic: t }]);
			await until("the push", () => got.length > 0);
			assert.equal(got.length, 1);
		} finally {
			p.stream.stop();
		}
	});

	it("reports a denied topic and keeps the granted one", async () => {
		const ok = topic("granted");
		const p = probe(url, [ok, "other:forbidden"]);
		const got: PushMessage[] = [];
		p.stream.onAny((m) => got.push(m));
		p.stream.start();
		try {
			await until("subscribed", () => p.subscribed.length > 0);
			assert.deepEqual(p.subscribed[0]!.granted, [ok]);
			assert.deepEqual(p.denied[0], [{ topic: "other:forbidden", code: "TOPIC_FORBIDDEN" }]);
			await publish(url, [{ seq: 1, topic: ok }]);
			await until("the push", () => got.length > 0);
		} finally {
			p.stream.stop();
		}
	});

	it("stops with NO_TOPICS when nothing is granted", async () => {
		// The ticket makes the client known; no rule or ticket grants the topic. (With no credential at all the
		// server answers 401, which the reader retries with backoff.)
		const p = probe(url, ["other:forbidden"]);
		p.stream.start();
		await until("stopped", () => p.stream.state === "stopped");
		assert.equal(p.errors.length, 1);
		assert.equal(p.errors[0]!.code, "NO_TOPICS");
		assert.equal(p.errors[0]!.status, 403);
		assert.equal(p.subscribed.length, 0);
		await sleep(500);
		assert.equal(p.errors.length, 1); // it did not retry
	});

	it("reconnects by itself after the server is killed, with no gap", { skip: restartCmd ? false : "FIELD_PUSH_E2E_RESTART_CMD is not set" }, async () => {
		const t = topic("restart");
		const p = probe(url, [t]);
		const got: PushMessage[] = [];
		p.stream.onAny((m) => got.push(m));
		p.stream.start();
		try {
			await until("subscribed", () => p.subscribed.length > 0);
			await publishSeq(url, t, 0, 10);
			await until("10 pushes", () => got.length >= 10);

			const r = spawnSync("sh", ["-c", restartCmd ?? "true"], { encoding: "utf8" });
			assert.equal(r.status, 0, `restart command failed: ${r.stderr}`);

			await until("reconnecting", () => p.states.includes("reconnecting"));
			await until("open again", () => p.subscribed.length >= 2 && p.stream.state === "open", 60_000);
			assert.ok(p.states.lastIndexOf("open") > p.states.indexOf("reconnecting"));

			await publishSeq(url, t, 10, 10);
			await until("20 pushes", () => got.length >= 20);
			await sleep(300);
			assert.deepEqual(seqs(got), range(0, 20));
			assert.ok(ascending(got));
			assert.deepEqual(p.resets, []);
		} finally {
			p.stream.stop();
		}
	});

	it(
		"resubscribes and resumes with no gap when the stream token expires",
		{ skip: shortBase ? false : "FIELD_PUSH_E2E_SHORT_URL is not set", timeout: 150_000 },
		async () => {
			const u = shortBase ?? "";
			const t = topic("expiry");
			const p = probe(u, [t]);
			const got: PushMessage[] = [];
			p.stream.onAny((m) => got.push(m));
			p.stream.start();
			try {
				await until("subscribed", () => p.subscribed.length > 0);
				// one push every 2 s for as long as it takes the 1-minute stream to expire and come back
				let n = 0;
				const end = Date.now() + 100_000;
				while (p.subscribed.length < 2 && Date.now() < end) {
					await publish(u, [{ seq: n++, topic: t }]);
					await sleep(2000);
				}
				assert.ok(p.subscribed.length >= 2, "the stream did not resubscribe within 100 s");
				await publish(u, [{ seq: n++, topic: t }]);
				await until("all pushes", () => got.length >= n);
				await sleep(300);
				assert.deepEqual(seqs(got), range(0, n));
				assert.ok(ascending(got));
				assert.deepEqual(p.resets, []);
				assert.deepEqual(p.errors, []);
			} finally {
				p.stream.stop();
			}
		},
	);
});
