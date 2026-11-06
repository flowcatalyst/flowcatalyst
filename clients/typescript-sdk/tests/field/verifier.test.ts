import { strict as assert } from "node:assert";
import { existsSync, readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { createLocalJWKSet, SignJWT, generateKeyPair } from "jose";
import {
	FieldError,
	FieldReason,
	FieldTokenVerifier,
	FieldVerifierUnavailableError,
} from "../../src/field/index.js";
import {
	APP,
	ISSUER,
	mintProof,
	mintToken,
	newDevice,
	newPlatform,
} from "./helpers.js";

const fixtureDir = fileURLToPath(
	new URL("../../../../api/field-fixtures/", import.meta.url),
);

async function reasonOf(p: Promise<unknown>): Promise<string> {
	try {
		await p;
	} catch (e) {
		if (e instanceof FieldError) return e.reason;
		throw e;
	}
	return "valid";
}

describe("shared conformance fixtures (api/field-fixtures)", () => {
	if (!existsSync(`${fixtureDir}cases.json`)) {
		it("SKIPPED: api/field-fixtures not present (SDK mirrored without it)", (t) => {
			t.skip("fixtures directory absent");
		});
		return;
	}
	const fx = JSON.parse(readFileSync(`${fixtureDir}cases.json`, "utf8")) as {
		issuer: string;
		appCode: string;
		cases: {
			name: string;
			accessToken: string;
			dpopProof: string;
			method: string;
			url: string;
			now: string;
			expect: string;
			principal?: {
				deviceId: string;
				deviceMode: string;
				amr: string[];
				actor?: { id: string; type: string; name: string };
				test: boolean;
				linkedPrincipalId: string;
			};
		}[];
	};
	const jwks = JSON.parse(readFileSync(`${fixtureDir}jwks.json`, "utf8"));
	assert.ok(fx.cases.length >= 20, "expected the full vector set");
	assert.ok(
		fx.cases.filter((c) => c.principal).length >= 4,
		"expected the sign-in-options principal cases",
	);
	assert.ok(
		fx.cases.some((c) => c.name === "valid_linked" && c.principal?.linkedPrincipalId),
		"expected the valid_linked case",
	);
	for (const c of fx.cases) {
		it(`${c.name} -> ${c.expect}`, async () => {
			const v = new FieldTokenVerifier({
				issuer: fx.issuer,
				appCode: fx.appCode,
				keys: createLocalJWKSet(jwks),
				now: () => new Date(c.now),
			});
			const got = await reasonOf(
				v.verify(`DPoP ${c.accessToken}`, c.dpopProof, c.method, c.url),
			);
			assert.equal(got, c.expect);
		});
		if (c.expect === "valid" && c.principal) {
			const want = c.principal;
			it(`${c.name} exposes the principal`, async () => {
				const v = new FieldTokenVerifier({
					issuer: fx.issuer,
					appCode: fx.appCode,
					keys: createLocalJWKSet(jwks),
					now: () => new Date(c.now),
				});
				const p = await v.verify(`DPoP ${c.accessToken}`, c.dpopProof, c.method, c.url);
				assert.equal(p.workerId, "fwu_0FIXTUREWORKER");
				assert.equal(p.workerCode, "W001");
				assert.equal(p.deviceId, want.deviceId);
				assert.equal(p.deviceMode, want.deviceMode);
				assert.deepEqual(p.amr, want.amr);
				assert.equal(p.test, want.test);
				assert.equal(p.linkedPrincipalId, want.linkedPrincipalId);
				assert.deepEqual(p.actor, want.actor ?? null);
			});
		}
	}
	it("valid case yields the principal", async () => {
		const c = fx.cases[0]!;
		const v = new FieldTokenVerifier({
			issuer: fx.issuer,
			appCode: fx.appCode,
			keys: createLocalJWKSet(jwks),
			now: () => new Date(c.now),
		});
		const p = await v.verify(`DPoP ${c.accessToken}`, c.dpopProof, c.method, c.url);
		assert.equal(p.workerId, "fwu_0FIXTUREWORKER");
		assert.equal(p.workerCode, "W001");
		assert.equal(p.appCode, fx.appCode);
		assert.deepEqual(p.amr, ["pin"]);
	});
});

describe("FieldTokenVerifier (generated tokens)", async () => {
	const platform = await newPlatform();
	const device = await newDevice();
	const URL_ = "https://app.example.test/api/orders";
	const mk = (extra: Partial<ConstructorParameters<typeof FieldTokenVerifier>[0]> = {}) =>
		new FieldTokenVerifier({ issuer: ISSUER, appCode: APP, keys: platform.keys, ...extra });

	it("accepts a good token + proof and ignores the query string", async () => {
		const token = await mintToken(platform, device);
		const proof = await mintProof(device, token, "POST", URL_);
		const p = await mk().verify(`DPoP ${token}`, proof, "POST", `${URL_}?x=1`);
		assert.equal(p.jkt, device.jkt);
		assert.equal(p.clientIdentifier, "acme");
	});

	it("a token with no device_id is an unenrolled key; no act claim means no actor", async () => {
		const token = await mintToken(platform, device, { device_id: undefined, device_mode: "UNENROLLED" });
		const proof = await mintProof(device, token, "POST", URL_);
		const p = await mk().verify(`DPoP ${token}`, proof, "POST", URL_);
		assert.equal(p.deviceId, "");
		assert.equal(p.deviceMode, "UNENROLLED");
		assert.equal(p.actor, null);
		assert.equal(p.test, false);
	});

	it("exposes act as actor and test", async () => {
		const token = await mintToken(platform, device, {
			act: { sub: "prn_1", typ: "principal", name: "Admin" },
			test: true,
		});
		const proof = await mintProof(device, token, "POST", URL_);
		const p = await mk().verify(`DPoP ${token}`, proof, "POST", URL_);
		assert.deepEqual(p.actor, { id: "prn_1", type: "principal", name: "Admin" });
		assert.equal(p.test, true);
		assert.equal(p.workerId, "fwu_1");
	});

	it("refuses HS256, none and ES256 access tokens as invalid_token", async () => {
		const device2 = device;
		const hs = await new SignJWT({ iss: ISSUER, aud: APP, token_use: "field" })
			.setProtectedHeader({ alg: "HS256", typ: "fc-field+jwt" })
			.sign(new TextEncoder().encode("0123456789abcdef0123456789abcdef"));
		const none =
			Buffer.from(JSON.stringify({ alg: "none", typ: "fc-field+jwt" })).toString("base64url") +
			"." +
			Buffer.from(JSON.stringify({ iss: ISSUER, aud: APP, token_use: "field" })).toString("base64url") +
			".";
		const { privateKey } = await generateKeyPair("ES256");
		const es = await new SignJWT({ iss: ISSUER, aud: APP, token_use: "field" })
			.setProtectedHeader({ alg: "ES256", typ: "fc-field+jwt" })
			.sign(privateKey);
		for (const t of [hs, none, es]) {
			const proof = await mintProof(device2, t, "POST", URL_);
			assert.equal(
				await reasonOf(mk().verify(`DPoP ${t}`, proof, "POST", URL_)),
				FieldReason.InvalidToken,
			);
		}
	});

	it("refuses a proof that is not ES256", async () => {
		const token = await mintToken(platform, device);
		const { privateKey } = await generateKeyPair("RS256");
		const bad = await new SignJWT({ htm: "POST", htu: URL_, jti: "x", iat: 1 })
			.setProtectedHeader({ alg: "RS256", typ: "dpop+jwt" })
			.sign(privateKey);
		assert.equal(
			await reasonOf(mk().verify(`DPoP ${token}`, bad, "POST", URL_)),
			FieldReason.InvalidProof,
		);
	});

	it("refuses a proof carrying a private key (d)", async () => {
		const token = await mintToken(platform, device);
		const hdr = Buffer.from(
			JSON.stringify({ alg: "ES256", typ: "dpop+jwt", jwk: { ...device.publicJwk, d: "AA" } }),
		).toString("base64url");
		const proof = `${hdr}.e30.${Buffer.alloc(64).toString("base64url")}`;
		assert.equal(
			await reasonOf(mk().verify(`DPoP ${token}`, proof, "POST", URL_)),
			FieldReason.InvalidProof,
		);
	});

	it("refuses a pcr token", async () => {
		const token = await mintToken(platform, device, { pcr: true });
		const proof = await mintProof(device, token, "POST", URL_);
		assert.equal(
			await reasonOf(mk().verify(`DPoP ${token}`, proof, "POST", URL_)),
			FieldReason.PCR,
		);
	});

	it("rejects a Bearer scheme as malformed", async () => {
		const token = await mintToken(platform, device);
		assert.equal(
			await reasonOf(mk().verify(`Bearer ${token}`, "x", "POST", URL_)),
			FieldReason.Malformed,
		);
	});

	it("proofMaxAge: stale by default, accepted when widened", async () => {
		const token = await mintToken(platform, device);
		const old = Math.floor(Date.now() / 1000) - 600;
		const proof = await mintProof(device, token, "POST", URL_, { iat: old });
		assert.equal(
			await reasonOf(mk().verify(`DPoP ${token}`, proof, "POST", URL_)),
			FieldReason.StaleProof,
		);
		const p = await mk({ proofMaxAgeSeconds: 1200 }).verify(`DPoP ${token}`, proof, "POST", URL_);
		assert.equal(p.workerCode, "W001");
	});

	it("an unreachable issuer is unavailable, not a refusal", async () => {
		const token = await mintToken(platform, device);
		const proof = await mintProof(device, token, "POST", URL_);
		const v = new FieldTokenVerifier({
			issuer: "http://127.0.0.1:1",
			appCode: APP,
			fetch: async () => {
				throw new TypeError("fetch failed");
			},
		});
		await assert.rejects(
			v.verify(`DPoP ${token}`, proof, "POST", URL_),
			FieldVerifierUnavailableError,
		);
	});
});
