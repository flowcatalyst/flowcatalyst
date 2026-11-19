/**
 * A platform token and a DPoP proof verify only in the spelling they were
 * issued in (docs/wire-contract.md, Authentication, "Token spelling"): every
 * re-spelling and every change in content is refused, and the issued value
 * still verifies afterwards. Covers the field verifier (minted tokens and the
 * shared vectors in api/field-fixtures/noncanonical.json) and the OIDC
 * client's access-token and ID-token verification (the platform's own tokens).
 */
import { strict as assert } from "node:assert";
import { existsSync, readFileSync } from "node:fs";
import { after, before, describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { createLocalJWKSet } from "jose";
import { createOidcClient } from "../../src/fastify/oidc/discovery.js";
import {
	decodeCanonicalSegment,
	isCanonicalCompact,
	isCanonicalSegment,
} from "../../src/field/compact-jws.js";
import { FieldError, FieldReason, FieldTokenVerifier } from "../../src/field/index.js";
import { startMockIssuer, type MockIssuer } from "../fastify/mock-issuer.js";
import { APP, ISSUER, mintProof, mintToken, newDevice, newPlatform } from "./helpers.js";
import { nonCanonicalSpellings, respellings, tampered } from "./respell.js";

const fixtureDir = fileURLToPath(new URL("../../../../api/field-fixtures/", import.meta.url));
const URL_ = "https://app.example.test/api/orders";

async function reasonOf(p: Promise<unknown>): Promise<string> {
	try {
		await p;
	} catch (e) {
		if (e instanceof FieldError) return e.reason;
		return "error";
	}
	return "valid";
}

describe("canonical base64url (compact-jws)", () => {
	it("accepts exactly what the encoder writes", () => {
		for (let n = 0; n < 10; n++) {
			const b = Buffer.from(Array.from({ length: n }, (_, i) => 0xff - i));
			const s = b.toString("base64url");
			assert.ok(isCanonicalSegment(s), `length ${n}: ${s}`);
			assert.deepEqual(decodeCanonicalSegment(s), b);
		}
		const sig = Buffer.from([0xfb, 0xff, 0xbf, 0xfe]).toString("base64url"); // "-_-_-g"
		const bad = nonCanonicalSpellings(sig);
		assert.equal(bad.length, 15);
		bad.push(`${sig}==`, `${sig}=`, "+/+/+g", "-_\n-_-g", "-_\r-_-g", `${sig}\r\n`, ` ${sig}`, `${sig} `, "A");
		for (const s of bad) {
			assert.equal(isCanonicalSegment(s), false, JSON.stringify(s));
			assert.equal(decodeCanonicalSegment(s), undefined);
		}
		assert.ok(isCanonicalCompact("eyJhbGciOiJIUzI1NiJ9.e30.AAAAAA"));
		for (const s of ["", "a.b", "a.b.c.d", "eyJhbGciOiJIUzI1NiJ9.e30.AAAAAB", "eyJhbGciOiJIUzI1NiJ9.e30=.AAAAAA", "eyJhbGciOiJIUzI1NiJ9.e30.AAAAAA\n"]) {
			assert.equal(isCanonicalCompact(s), false, JSON.stringify(s));
		}
	});
});

describe("FieldTokenVerifier: only the issued spelling verifies", async () => {
	const platform = await newPlatform();
	const device = await newDevice();
	const v = new FieldTokenVerifier({ issuer: ISSUER, appCode: APP, keys: platform.keys });
	const token = await mintToken(platform, device, { sub: "fwu_canonical" });
	const proof = await mintProof(device, token, "POST", URL_, { jti: "proof-jti-1" });
	const ok = async (t: string, p: string) => reasonOf(v.verify(`DPoP ${t}`, p, "POST", URL_));

	it("the issued token and proof verify", async () => {
		assert.equal(await ok(token, proof), "valid");
	});
	// Whitespace around the token is the Authorization header's framing, not the token.
	const tokenCases = respellings(token, false);
	it("a 2048-bit RS256 signature has 15 other spellings of its last character", () => {
		assert.equal(tokenCases.size, 15 + 6);
	});
	for (const [name, bad] of tokenCases) {
		it(`token re-spelled (${name}) is malformed, with a proof made for its exact text`, async () => {
			assert.notEqual(bad, token);
			const p = await mintProof(device, bad, "POST", URL_);
			assert.equal(await ok(bad, p), FieldReason.Malformed);
		});
	}
	for (const [name, bad] of tampered(token, "fwu_canonical", "fwu_somebody")) {
		it(`token tampered (${name}) is refused`, async () => {
			const p = await mintProof(device, bad, "POST", URL_);
			assert.notEqual(await ok(bad, p), "valid");
		});
	}
	const proofCases = respellings(proof, true);
	it("a 64-byte ES256 signature has 15 other spellings of its last character", () => {
		assert.equal(proofCases.size, 15 + 6 + 3);
	});
	for (const [name, bad] of proofCases) {
		it(`proof re-spelled (${name}) is invalid_proof`, async () => {
			assert.notEqual(bad, proof);
			assert.equal(await ok(token, bad), FieldReason.InvalidProof);
		});
	}
	for (const [name, bad] of tampered(proof, "proof-jti-1", "proof-jti-2")) {
		it(`proof tampered (${name}) is refused`, async () => {
			assert.notEqual(await ok(token, bad), "valid");
		});
	}
	it("the issued token and proof still verify", async () => {
		assert.equal(await ok(token, proof), "valid");
	});
});

describe("shared non-canonical vectors (api/field-fixtures/noncanonical.json)", () => {
	if (!existsSync(`${fixtureDir}noncanonical.json`)) {
		it("SKIPPED: api/field-fixtures not present (SDK mirrored without it)", (t) => {
			t.skip("fixtures directory absent");
		});
		return;
	}
	const fx = JSON.parse(readFileSync(`${fixtureDir}noncanonical.json`, "utf8")) as {
		issuer: string;
		appCode: string;
		cases: { name: string; accessToken: string; dpopProof: string; method: string; url: string; now: string; expect: string }[];
	};
	const jwks = JSON.parse(readFileSync(`${fixtureDir}jwks.json`, "utf8"));
	assert.ok(fx.cases.length > 50, "expected the full vector set");
	for (const c of fx.cases) {
		it(`${c.name} -> ${c.expect}`, async () => {
			assert.notEqual(c.expect, "valid");
			const v = new FieldTokenVerifier({
				issuer: fx.issuer,
				appCode: fx.appCode,
				keys: createLocalJWKSet(jwks),
				now: () => new Date(c.now),
			});
			assert.equal(await reasonOf(v.verify(`DPoP ${c.accessToken}`, c.dpopProof, c.method, c.url)), c.expect);
		});
	}
});

describe("OIDC client: platform access and ID tokens verify only as issued", () => {
	let issuer: MockIssuer;
	before(async () => {
		issuer = await startMockIssuer();
	});
	after(async () => {
		await issuer.stop();
	});

	const check = async (token: string, verify: (t: string) => Promise<unknown>) => {
		const accepts = async (t: string) => {
			try {
				await verify(t);
				return true;
			} catch {
				return false;
			}
		};
		assert.ok(await accepts(token), "the issued token must verify");
		const cases = respellings(token, true);
		assert.equal(cases.size, 15 + 6 + 3);
		for (const [name, bad] of cases) {
			assert.notEqual(bad, token);
			assert.equal(await accepts(bad), false, `re-spelled (${name}) must be refused`);
		}
		for (const [name, bad] of tampered(token, "prn_canonical", "prn_somebody")) {
			assert.equal(await accepts(bad), false, `tampered (${name}) must be refused`);
		}
		assert.ok(await accepts(token), "the issued token must still verify");
	};

	it("access tokens (verify)", async () => {
		const endpoints = await createOidcClient({ baseUrl: issuer.baseUrl }).endpoints();
		const token = await issuer.signAccessToken({ sub: "prn_canonical", name: "Canon" });
		await check(token, (t) => endpoints.verify(t));
	});

	it("ID tokens (verifyIdToken)", async () => {
		const endpoints = await createOidcClient({ baseUrl: issuer.baseUrl }).endpoints();
		const token = await issuer.signIdToken({ sub: "prn_canonical" });
		await check(token, (t) => endpoints.verifyIdToken(t, "test-client"));
	});
});
