import { strict as assert } from "node:assert";
import { after, before, describe, it } from "node:test";
import Fastify, { type FastifyInstance } from "fastify";
import fp from "fastify-plugin";
import { fieldAuth, flowcatalystAuth, generateSessionSecret } from "../../src/fastify/index.js";
import { startMockIssuer, type MockIssuer } from "../fastify/mock-issuer.js";
import { APP, ISSUER, mintProof, mintToken, newDevice, newPlatform, type Device, type Platform } from "./helpers.js";

const PUBLIC = "https://app.example.test";

let app: FastifyInstance;
let issuer: MockIssuer;
let platform: Platform;
let device: Device;

before(async () => {
	platform = await newPlatform();
	device = await newDevice();
	issuer = await startMockIssuer();
	app = Fastify({ logger: false });

	// OIDC plane at root context (fp-wrapped): the dual-registration shape.
	await app.register(fp(flowcatalystAuth), {
		baseUrl: issuer.baseUrl,
		clientId: "clt_x",
		clientSecret: "s",
		cookie: { secret: generateSessionSecret(), secure: false },
	});
	await app.register(fieldAuth, {
		issuer: ISSUER,
		appCode: APP,
		keys: platform.keys,
		publicBaseUrl: PUBLIC,
	});

	app.post("/api/orders", { preHandler: app.fcField.require() }, async (req) => ({
		worker: req.fieldPrincipal!.workerCode,
		device: req.fieldPrincipal!.deviceId,
		principal: req.principal ?? null,
	}));
	app.get("/open", async (req) => ({
		field: req.fieldPrincipal ?? null,
	}));
	app.get("/bearer", { preHandler: app.fc.requireBearer() }, async (req) => ({
		id: req.principal!.id,
	}));
	app.post("/api/orders/both", { preHandler: [app.fc.requireAuth()] }, async (req) => ({
		id: req.principal!.id,
		field: req.fieldPrincipal ?? null,
	}));
	await app.ready();
});

after(async () => {
	await app.close();
	await issuer.stop();
});

async function signed(over: Record<string, unknown> = {}, path = "/api/orders") {
	const token = await mintToken(platform, device, over);
	const proof = await mintProof(device, token, "POST", PUBLIC + path);
	return { token, proof };
}

describe("fieldAuth (Fastify)", () => {
	it("happy path sets fieldPrincipal and leaves request.principal alone", async () => {
		const { token, proof } = await signed();
		const res = await app.inject({
			method: "POST",
			url: "/api/orders?x=1",
			headers: { authorization: `DPoP ${token}`, dpop: proof, host: "evil.test" },
		});
		assert.equal(res.statusCode, 200);
		assert.deepEqual(res.json(), { worker: "W001", device: "fwd_1", principal: null });
	});

	it("htu comes from publicBaseUrl, never the Host header", async () => {
		const token = await mintToken(platform, device);
		const proof = await mintProof(device, token, "POST", "https://evil.test/api/orders");
		const res = await app.inject({
			method: "POST",
			url: "/api/orders",
			headers: { authorization: `DPoP ${token}`, dpop: proof, host: "evil.test" },
		});
		assert.equal(res.statusCode, 401);
		assert.equal(res.json().error, "htu_mismatch");
	});

	it("missing credentials -> 401 with WWW-Authenticate DPoP", async () => {
		const res = await app.inject({ method: "POST", url: "/api/orders" });
		assert.equal(res.statusCode, 401);
		assert.equal(res.headers["www-authenticate"], 'DPoP error="invalid_token"');
		assert.equal(res.json().error, "malformed");
	});

	it("missing proof -> 401 missing_proof", async () => {
		const { token } = await signed();
		const res = await app.inject({
			method: "POST",
			url: "/api/orders",
			headers: { authorization: `DPoP ${token}` },
		});
		assert.equal(res.statusCode, 401);
		assert.equal(res.json().error, "missing_proof");
	});

	it("pcr -> 403 PIN_CHANGE_REQUIRED", async () => {
		const { token, proof } = await signed({ pcr: true });
		const res = await app.inject({
			method: "POST",
			url: "/api/orders",
			headers: { authorization: `DPoP ${token}`, dpop: proof },
		});
		assert.equal(res.statusCode, 403);
		assert.equal(res.json().error, "PIN_CHANGE_REQUIRED");
	});

	it("only runs on opted-in routes", async () => {
		const res = await app.inject({ method: "GET", url: "/open" });
		assert.equal(res.statusCode, 200);
		assert.equal(res.json().field, null);
	});

	it("does not clobber a request.principal resolved by the OIDC plane", async () => {
		const bearer = await issuer.signAccessToken({ sub: "prn_admin", name: "Admin" });
		// A DPoP-scheme header would be a field credential; the OIDC plane only
		// reads Bearer, so exercise the bearer-only route and the field route
		// side by side on the same app.
		const b = await app.inject({
			method: "GET",
			url: "/bearer",
			headers: { authorization: `Bearer ${bearer}` },
		});
		assert.equal(b.statusCode, 200);
		assert.equal(b.json().id, "prn_admin");

		const { token, proof } = await signed();
		const f = await app.inject({
			method: "POST",
			url: "/api/orders",
			headers: { authorization: `DPoP ${token}`, dpop: proof },
		});
		assert.equal(f.statusCode, 200);
		assert.equal(f.json().principal, null);
	});

	it("a pre-resolved request.principal survives the field preHandler", async () => {
		const sub = Fastify({ logger: false });
		await sub.register(fieldAuth, {
			issuer: ISSUER,
			appCode: APP,
			keys: platform.keys,
			publicBaseUrl: PUBLIC,
		});
		sub.decorateRequest("principal", undefined);
		sub.addHook("onRequest", async (req) => {
			(req as unknown as { principal: unknown }).principal = { id: "sentinel" };
		});
		sub.post("/api/orders", { preHandler: sub.fcField.require() }, async (req) => ({
			principal: (req as unknown as { principal: unknown }).principal,
			worker: req.fieldPrincipal!.workerCode,
		}));
		const { token, proof } = await signed();
		const res = await sub.inject({
			method: "POST",
			url: "/api/orders",
			headers: { authorization: `DPoP ${token}`, dpop: proof },
		});
		assert.equal(res.statusCode, 200);
		assert.deepEqual(res.json(), { principal: { id: "sentinel" }, worker: "W001" });
		await sub.close();
	});
});

describe("platform bearer validator token_use allowlist", () => {
	const bearerStatus = async (extra: Record<string, unknown>) => {
		const t = await issuer.signAccessToken({
			sub: "prn_1",
			name: "N",
			...(extra as object),
		});
		const res = await app.inject({
			method: "GET",
			url: "/bearer",
			headers: { authorization: `Bearer ${t}` },
		});
		return res.statusCode;
	};
	it("accepts token_use api, identity and absent", async () => {
		assert.equal(await bearerStatus({}), 200);
		assert.equal(await bearerStatus({ token_use: "api" }), 200);
		assert.equal(await bearerStatus({ token_use: "identity" }), 200);
	});
	it("refuses field and unknown token_use", async () => {
		assert.equal(await bearerStatus({ token_use: "field" }), 401);
		assert.equal(await bearerStatus({ token_use: "portal" }), 401);
	});
});
