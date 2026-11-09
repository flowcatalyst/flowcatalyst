import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createPublicKey, verify as edVerify } from "node:crypto";
import { fileURLToPath } from "node:url";

import {
	TicketError,
	TicketKey,
	TicketMinter,
	type TicketJwks,
} from "../src/push/tickets.js";

// conformance/push/tickets.json of the framework (pinned copy). The keys in it are test keys,
// published on purpose; EdDSA is deterministic, so the SDK helper must reproduce a valid ticket
// byte for byte, and its output must verify against the vector's own expectations.
const vector = JSON.parse(
	readFileSync(fileURLToPath(new URL("./fixtures/push/tickets.json", import.meta.url)), "utf8"),
) as {
	application: string;
	now: number;
	maxLifetimeSeconds: number;
	keys: Record<"current" | "previous" | "stranger", { kid: string; seed: string; x: string }>;
	jwks: TicketJwks;
	cases: Array<{
		name: string;
		tickets: string[];
		topics: string[];
		context: { client: string; principal: string; proofKey: string | null } | null;
		expect: { ticketReasons?: Array<string | null>; error?: string };
	}>;
};

const b64uJson = (s: string): Record<string, unknown> =>
	JSON.parse(Buffer.from(s, "base64url").toString("utf8")) as Record<string, unknown>;

function decode(token: string) {
	const [h, c, s] = token.split(".");
	if (!h || !c || s === undefined) return null;
	try {
		return { header: b64uJson(h), claims: b64uJson(c), signingInput: `${h}.${c}`, sig: Buffer.from(s, "base64url") };
	} catch {
		return null;
	}
}

/**
 * A ticket checker per push contract 6.4 for a host that knows one application and the JWKS
 * (the vector's "ticketReasons" words), used to show the vector's tickets and the helper's tickets
 * are judged alike.
 */
function reasonOf(
	token: string,
	jwks: TicketJwks,
	now: number,
	ctx: { client: string | null; proofKey: string | null },
	streamClient: { value: string | null },
): string | null {
	const t = decode(token);
	if (!t) return "malformed";
	if (t.header["alg"] !== "EdDSA") return "alg";
	if (t.claims["iss"] !== vector.application) return "iss";
	const jwk = jwks.keys.find((k) => k.kid === t.header["kid"]);
	if (!jwk) return "kid";
	const pub = createPublicKey({ key: { kty: "OKP", crv: "Ed25519", x: jwk.x }, format: "jwk" });
	if (!edVerify(null, Buffer.from(t.signingInput, "ascii"), pub, t.sig)) return "signature";
	if (t.claims["aud"] !== "fw-push") return "aud";
	const iat = t.claims["iat"] as number;
	const exp = t.claims["exp"] as number;
	if (exp <= iat || exp - iat > 300) return "times";
	if (exp <= now) return "expired";
	const topics = t.claims["topics"] as string[];
	if (topics.some((p) => p.split(":")[0] !== vector.application)) return "topic-outside-iss";
	const cnf = t.claims["cnf"] as { jkt?: string } | undefined;
	if (cnf?.jkt && cnf.jkt !== ctx.proofKey) return "cnf";
	const client = t.claims["client"] as string;
	const want = ctx.client ?? streamClient.value;
	if (want !== null && client !== want) return "client-mismatch";
	if (streamClient.value === null) streamClient.value = client;
	return null;
}

function reasonsFor(c: (typeof vector.cases)[number], jwks: TicketJwks, tickets = c.tickets) {
	const stream = { value: null as string | null };
	return tickets.map((tk) =>
		reasonOf(tk, jwks, vector.now, { client: c.context?.client ?? null, proofKey: c.context?.proofKey ?? null }, stream),
	);
}

const seeds = [vector.keys.current, vector.keys.previous, vector.keys.stranger];

describe("tickets.json conformance", () => {
	test("the JWKS from the configured keys is the vector's, current then previous", () => {
		const m = TicketMinter.fromConfig(
			vector.application,
			`${vector.keys.current.kid}:${vector.keys.current.seed}`,
			[`${vector.keys.previous.kid}:${vector.keys.previous.seed}`],
		);
		assert.deepEqual(m.jwks(), vector.jwks);
		// a public JWK is enough for a previous key
		const m2 = new TicketMinter(vector.application, TicketKey.parse(`${vector.keys.current.kid}:${vector.keys.current.seed}`), [
			vector.jwks.keys[1]!,
		]);
		assert.deepEqual(m2.jwks(), vector.jwks);
	});

	test("the vector's tickets are judged as the vector says (checker validated on the vector)", () => {
		let checked = 0;
		for (const c of vector.cases) {
			if (!c.expect.ticketReasons) continue;
			assert.deepEqual(reasonsFor(c, vector.jwks), c.expect.ticketReasons, c.name);
			checked++;
		}
		assert.ok(checked >= 15, `checked ${checked} cases`);
	});

	test("the helper reproduces the vector's tickets byte for byte, and refuses what it must not mint", () => {
		let reproduced = 0;
		const refusedReasons: string[] = [];
		for (const c of vector.cases) {
			c.tickets.forEach((token, i) => {
				const t = decode(token);
				if (!t || t.header["alg"] !== "EdDSA") return;
				const reason = c.expect.ticketReasons?.[i];
				const claims = t.claims as {
					iss: string; aud: string; client: string; topics: string[];
					sub?: string; cnf?: { jkt: string }; iat: number; exp: number; "stream-exp"?: number;
				};
				let minted: string | null = null;
				for (const s of seeds) {
					// the key bound to the ticket's kid, whichever seed signed it
					const key = TicketKey.fromSeed(String(t.header["kid"]), s.seed);
					try {
						const m = new TicketMinter(claims.iss, key);
						const out = m.mint({
							client: claims.client,
							topics: claims.topics,
							sub: claims.sub,
							jkt: claims.cnf?.jkt,
							now: claims.iat,
							ttlSeconds: claims.exp - claims.iat,
							streamExp: claims["stream-exp"],
						});
						if (out === token) {
							minted = out;
							break;
						}
					} catch (e) {
						assert.ok(e instanceof TicketError);
						refusedReasons.push(reason ?? "?");
						return;
					}
				}
				if (reason === null) {
					assert.equal(minted, token, `${c.name}: a valid ticket is reproduced`);
				}
				if (minted) reproduced++;
				else assert.notEqual(reason, null, `${c.name}: only an invalid ticket is out of the helper's reach`);
			});
		}
		assert.ok(reproduced >= 12, `reproduced ${reproduced}`);
		// exp more than 5 minutes after iat, and a topic outside iss, cannot be minted at all
		assert.ok(refusedReasons.includes("times"), refusedReasons.join());
		assert.ok(refusedReasons.includes("topic-outside-iss"), refusedReasons.join());
	});

	test("tickets the helper mints at the vector's time verify with the vector's expectations", () => {
		const current = `${vector.keys.current.kid}:${vector.keys.current.seed}`;
		const previous = `${vector.keys.previous.kid}:${vector.keys.previous.seed}`;
		const m = TicketMinter.fromConfig(vector.application, current, [previous]);
		const jwks = m.jwks();
		const stream = (client: string | null, proofKey: string | null = null) => ({
			ctx: { client, proofKey },
			s: { value: null as string | null },
		});

		const t1 = m.mint({ client: "acme", topics: ["fulfil:order:{id}"], sub: "prn_1", now: vector.now - 10 });
		const a = stream(null);
		assert.equal(reasonOf(t1, jwks, vector.now, a.ctx, a.s), null);
		assert.equal(a.s.value, "acme", "the first valid ticket names the stream's client");

		// another client than the stream's
		const b = stream("other");
		assert.equal(reasonOf(t1, jwks, vector.now, b.ctx, b.s), "client-mismatch");

		// bound to a DPoP key
		const t2 = m.mint({ client: "acme", topics: ["fulfil:order:{id}"], jkt: "jkt-A", now: vector.now - 10 });
		const c1 = stream("acme", "jkt-A");
		assert.equal(reasonOf(t2, jwks, vector.now, c1.ctx, c1.s), null);
		const c2 = stream("acme", "jkt-B");
		assert.equal(reasonOf(t2, jwks, vector.now, c2.ctx, c2.s), "cnf");

		// platform-wide: the platform client's value
		const t3 = m.mint({ client: "_platform", topics: ["fulfil:order:{id}"], now: vector.now - 10 });
		const d = stream(null);
		assert.equal(reasonOf(t3, jwks, vector.now, d.ctx, d.s), null);
		assert.equal(d.s.value, "_platform");

		// expired, and after the rotation the old key's tickets still verify while published
		const t4 = m.mint({ client: "acme", topics: ["fulfil:order:{id}"], ttlSeconds: 60, now: vector.now - 100 });
		const e = stream("acme");
		assert.equal(reasonOf(t4, jwks, vector.now, e.ctx, e.s), "expired");
		const old = TicketMinter.fromConfig(vector.application, previous);
		const t5 = old.mint({ client: "acme", topics: ["fulfil:order:{id}"], now: vector.now });
		const f = stream("acme");
		assert.equal(reasonOf(t5, jwks, vector.now, f.ctx, f.s), null, "signed with the previous key");
		const g = stream("acme");
		assert.equal(
			reasonOf(t5, TicketMinter.fromConfig(vector.application, current).jwks(), vector.now, g.ctx, g.s),
			"kid",
			"the previous key dropped from the JWKS: unknown kid",
		);

		// header and claims order are the contract's (the vector's tickets decode alike)
		const d1 = decode(t1)!;
		assert.deepEqual(Object.keys(d1.header), ["alg", "kid"]);
		assert.deepEqual(Object.keys(d1.claims), ["iss", "aud", "client", "topics", "sub", "iat", "exp"]);
		assert.equal(d1.claims["aud"], "fw-push");
		const d2 = decode(m.mint({ client: "acme", topics: ["fulfil:a:{x}"], sub: "p", jkt: "k", streamExp: new Date((vector.now + 60) * 1000), now: vector.now }))!;
		assert.deepEqual(Object.keys(d2.claims), ["iss", "aud", "client", "topics", "sub", "cnf", "iat", "exp", "stream-exp"]);
		assert.equal(d2.claims["stream-exp"], vector.now + 60);
	});
});

describe("ticket helper rules", () => {
	const m = TicketMinter.fromConfig("shop", `k1:${vector.keys.current.seed}`);
	const ok = { client: "acme", topics: ["shop:orders:{id}"] };

	test("refuses a topic of another application, no topics, too many topics, bad ttl, bad client", () => {
		assert.throws(() => m.mint({ ...ok, topics: ["orders:order:{id}"] }), TicketError);
		assert.throws(() => m.mint({ ...ok, topics: ["shop"] }), TicketError);
		assert.throws(() => m.mint({ ...ok, topics: [] }), TicketError);
		assert.throws(
			() => m.mint({ ...ok, topics: Array.from({ length: 101 }, (_, i) => `shop:t:${i}`) }),
			TicketError,
		);
		assert.doesNotThrow(() => m.mint({ ...ok, topics: Array.from({ length: 100 }, (_, i) => `shop:t:${i}`) }));
		assert.throws(() => m.mint({ ...ok, ttlSeconds: 301 }), TicketError);
		assert.throws(() => m.mint({ ...ok, ttlSeconds: 0 }), TicketError);
		assert.doesNotThrow(() => m.mint({ ...ok, ttlSeconds: 300 }));
		assert.throws(() => m.mint({ ...ok, client: "a b" }), TicketError);
	});

	test("key configuration: kid:seed only, 32-byte seed", () => {
		assert.throws(() => TicketKey.parse("noseed"), TicketError);
		assert.throws(() => TicketKey.parse(":abc"), TicketError);
		assert.throws(() => TicketKey.parse("k:AAAA"), TicketError);
		assert.throws(() => TicketKey.parse(`${"k".repeat(65)}:${vector.keys.current.seed}`), TicketError);
	});

	test("the JWKS carries no private member", () => {
		const j = m.jwks();
		assert.deepEqual(Object.keys(j.keys[0]!).sort(), ["alg", "crv", "kid", "kty", "use", "x"]);
	});

	test("the default lifetime is 60 seconds", () => {
		const t = decode(m.mint({ ...ok, now: 1000 }))!;
		assert.equal((t.claims["exp"] as number) - (t.claims["iat"] as number), 60);
	});
});
