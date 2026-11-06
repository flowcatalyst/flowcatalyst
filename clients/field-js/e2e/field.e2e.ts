// End-to-end against a real platform. It compiles and spawns the Go helper
// TestFieldJSE2EServer (internal/platform/fieldauth/e2eserver_pg_test.go): the
// real /field/* routes on an embedded Postgres, plus an application endpoint
// that verifies requests with the Go SDK's FieldTokenVerifier. Skips when Go
// or the embedded Postgres is unavailable. Kills only the child it spawned.

import { strict as assert } from "node:assert";
import { spawn, spawnSync, type ChildProcessWithoutNullStreams } from "node:child_process";
import { createServer } from "node:http";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { after, before, describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import {
	FieldAuth,
	FieldAuthError,
	MemoryDb,
	MemorySecureStore,
	WebCryptoKeyProvider,
	STORE_REFRESH,
} from "../src/index.js";

const repoRoot = resolve(fileURLToPath(new URL("../../..", import.meta.url)));
const pkgDir = join(repoRoot, "internal/platform/fieldauth");

interface Seed {
	baseUrl: string;
	appUrl: string;
	code: string;
	workerCode: string;
	pin: string;
	newPin: string;
	appCode: string;
}

let skipReason: string | null = null;
let child: ChildProcessWithoutNullStreams | null = null;
let workdir: string | null = null;
let seed: Seed | null = null;
let output = "";

function startServer(binary: string): Promise<Seed> {
	return new Promise((resolveSeed, reject) => {
		const c = spawn(binary, ["-test.run", "^TestFieldJSE2EServer$", "-test.timeout", "0", "-test.count", "1"], {
			cwd: pkgDir,
			env: { ...process.env, FC_FIELDJS_E2E: "1" },
			stdio: ["pipe", "pipe", "pipe"],
		});
		child = c;
		const timer = setTimeout(() => reject(new Error(`the platform did not start within 180 s\n${output}`)), 180_000);
		let buf = "";
		const onData = (d: Buffer) => {
			const text = d.toString();
			output = (output + text).slice(-8000);
			buf += text;
			let i: number;
			while ((i = buf.indexOf("\n")) >= 0) {
				const line = buf.slice(0, i);
				buf = buf.slice(i + 1);
				if (line.startsWith("FCFIELD_E2E ")) {
					clearTimeout(timer);
					resolveSeed(JSON.parse(line.slice("FCFIELD_E2E ".length)) as Seed);
				}
			}
		};
		c.stdout.on("data", onData);
		c.stderr.on("data", onData);
		c.on("exit", (code) => {
			clearTimeout(timer);
			reject(new Error(`the platform exited (${code}) before it was ready\n${output}`));
		});
	});
}

async function stopServer(): Promise<void> {
	const c = child;
	if (!c || c.exitCode !== null) return;
	const exited = new Promise<void>((r) => c.once("exit", () => r()));
	c.stdin.end(); // the helper returns when stdin closes, so testpg shuts Postgres down cleanly
	const t1 = setTimeout(() => c.kill("SIGTERM"), 30_000);
	const t2 = setTimeout(() => c.kill("SIGKILL"), 45_000);
	await exited;
	clearTimeout(t1);
	clearTimeout(t2);
}


// ── scenario helpers: each scenario gets its own client, application and keys ──

interface Scenario {
	scenario: string;
	clientIdentifier: string;
	appCode: string;
}

async function post<T>(path: string, body: unknown = {}): Promise<T> {
	assert.ok(seed);
	const r = await fetch(`${seed.baseUrl}${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
	assert.equal(r.status, 200, `${path}: ${await r.clone().text()}`);
	return (await r.json()) as T;
}

const newScenario = () => post<Scenario>("/e2e/new");
const op = (sc: Scenario, o: Record<string, unknown>) => post<Record<string, string>>("/e2e/op", { scenario: sc.scenario, ...o });

function mkAuth(sc: Scenario, extra: { clientIdentifier?: string } = {}): FieldAuth {
	assert.ok(seed);
	return new FieldAuth({
		platformUrl: seed.baseUrl,
		appCode: sc.appCode,
		keys: new WebCryptoKeyProvider({ db: new MemoryDb() }),
		store: new MemorySecureStore(),
		...extra,
	});
}

async function enrolled(sc: Scenario, extra: { clientIdentifier?: string } = {}): Promise<FieldAuth> {
	assert.ok(seed);
	const a = mkAuth(sc, extra);
	const { code } = await op(sc, { op: "enrolCode" });
	await a.enrol(`fcfield:1:${seed.baseUrl}:${code}`, { label: "e2e", deviceInfo: { platform: "web", model: "node" } });
	return a;
}

const PIN = "482915";
const PIN2 = "739204";

function code(c: string) {
	return (e: unknown) => {
		assert.ok(e instanceof FieldAuthError, `expected a FieldAuthError, got ${String(e)}`);
		assert.equal(e.code, c);
		return true;
	};
}

async function whoami(a: FieldAuth, sc: Scenario): Promise<{ status: number; body: Record<string, unknown> }> {
	assert.ok(seed);
	const r = await a.fetch(`${seed.appUrl}/app/whoami?app=${sc.appCode}`);
	const text = await r.text();
	return { status: r.status, body: r.status === 200 ? (JSON.parse(text) as Record<string, unknown>) : { raw: text } };
}

function amrOf(token: string): unknown {
	const payload = token.split(".")[1] ?? "";
	return (JSON.parse(Buffer.from(payload, "base64url").toString("utf8")) as { amr?: unknown }).amr;
}

// The access token the library is currently using, captured from its DPoP
// authorization header on a request to a throwaway local server.
async function accessTokenOf(a: FieldAuth): Promise<string> {
	return new Promise<string>((resolveTok, reject) => {
		const srv = createServer((req, res) => {
			res.end("ok");
			const h = req.headers["authorization"] ?? "";
			srv.close();
			resolveTok(String(h).replace(/^DPoP /, ""));
		});
		srv.listen(0, "127.0.0.1", () => {
			const port = (srv.address() as { port: number }).port;
			a.fetch(`http://127.0.0.1:${port}/x`).catch(reject);
		});
	});
}

// Run one scenario test (skips when the platform is unavailable).
function scenarioTest(name: string, fn: (sc: Scenario) => Promise<void>): void {
	it(name, async (t) => {
		if (skipReason || !seed) {
			t.skip(`SKIPPED: ${skipReason ?? "no server"}`);
			return;
		}
		await fn(await newScenario());
	});
}

describe("field-js against a real platform", () => {
	before(async () => {
		if (spawnSync("go", ["version"], { encoding: "utf8" }).status !== 0) {
			skipReason = "Go is not installed";
			return;
		}
		workdir = mkdtempSync(join(tmpdir(), "fc-fieldjs-e2e-"));
		const binary = join(workdir, "fieldauth.test");
		const build = spawnSync("go", ["test", "-c", "-tags=integration", "-o", binary, "./internal/platform/fieldauth"], {
			cwd: repoRoot,
			encoding: "utf8",
		});
		if (build.status !== 0) {
			throw new Error(`could not compile the platform test helper:\n${build.stdout}${build.stderr}`);
		}
		try {
			seed = await startServer(binary);
		} catch (e) {
			const msg = String(e);
			if (/testpg:|embedded postgres|initdb|postgres/i.test(msg)) {
				skipReason = `Postgres (testpg) is unavailable: ${msg.slice(0, 600)}`;
				return;
			}
			throw e;
		}
	});

	after(async () => {
		await stopServer();
		if (workdir) rmSync(workdir, { recursive: true, force: true });
	});

	it("enrol, login, fetch, refresh, changePin, login with the new PIN, logout", async (t) => {
		if (skipReason || !seed) {
			t.skip(`SKIPPED: ${skipReason ?? "no server"}`);
			console.log(`field-js e2e skipped: ${skipReason}`);
			return;
		}
		const s = seed;
		const store = new MemorySecureStore();
		const auth = new FieldAuth({
			platformUrl: s.baseUrl,
			appCode: s.appCode,
			keys: new WebCryptoKeyProvider({ db: new MemoryDb() }),
			store,
		});

		const dev = await auth.enrol(`fcfield:1:${s.baseUrl}:${s.code}`, { label: "e2e", deviceInfo: { platform: "web", model: "node" } });
		assert.match(dev.deviceId, /^fwd_/);
		assert.equal(dev.mode, "PERSONAL");
		assert.deepEqual(dev.appCodes, [s.appCode]);

		await assert.rejects(auth.login({ workerCode: s.workerCode, pin: "000000" }), (e: unknown) => e instanceof FieldAuthError && e.code === "INVALID_CREDENTIALS");

		const session = await auth.login({ workerCode: s.workerCode, pin: s.pin });
		assert.equal(session.worker.workerCode, s.workerCode.toUpperCase());
		assert.equal(session.pinChangeRequired, false);

		const who = await auth.fetch(`${s.appUrl}/app/whoami?x=1`);
		assert.equal(who.status, 200, await who.clone().text());
		const body = (await who.json()) as { workerCode: string; appCode: string; deviceId: string };
		assert.equal(body.workerCode, s.workerCode.toUpperCase());
		assert.equal(body.appCode, s.appCode);
		assert.equal(body.deviceId, dev.deviceId);

		const before = await store.get(STORE_REFRESH);
		const refreshed = await auth.refresh();
		assert.ok(refreshed.expiresAt > 0);
		assert.notEqual(await store.get(STORE_REFRESH), before, "the refresh token rotated");
		assert.equal((await auth.fetch(`${s.appUrl}/app/whoami`)).status, 200);

		await assert.rejects(auth.changePin({ currentPin: s.pin, newPin: "111111" }), (e: unknown) => e instanceof FieldAuthError && e.code === "PIN_POLICY");
		await auth.changePin({ currentPin: s.pin, newPin: s.newPin });

		await auth.logout();
		assert.equal(auth.session, null);

		await assert.rejects(auth.login({ workerCode: s.workerCode, pin: s.pin }), (e: unknown) => e instanceof FieldAuthError && e.code === "INVALID_CREDENTIALS");
		await auth.login({ workerCode: s.workerCode, pin: s.newPin });
		assert.equal((await auth.fetch(`${s.appUrl}/app/whoami`)).status, 200);

		// A restart: a new instance continues the session from the store.
		const again = new FieldAuth({
			platformUrl: s.baseUrl,
			appCode: s.appCode,
			keys: new WebCryptoKeyProvider({ db: new MemoryDb() }),
			store,
		});
		await again.restore();
		assert.equal(again.session?.worker.workerCode, s.workerCode.toUpperCase());

		await auth.logout({ scope: "device-all" });
		await assert.rejects(auth.refresh(), (e: unknown) => e instanceof FieldAuthError && e.code === "SESSION_EXPIRED");
	});

	// ── sign-in options (spec sections 1, 3, 4, 9) ─────────────────────────

	scenarioTest("unenrolled login with a client identifier; refresh; device-all refused locally", async (sc) => {
		await op(sc, { op: "worker", code: "un-1", pin: PIN });
		const a = mkAuth(sc, { clientIdentifier: sc.clientIdentifier });
		const session = await a.login({ workerCode: "un-1", pin: PIN });
		assert.equal(session.deviceMode, "UNENROLLED");
		assert.equal(session.deviceId, null);
		assert.equal(session.actor, null);
		assert.equal(session.test, false);
		const who = await whoami(a, sc);
		assert.equal(who.status, 200, JSON.stringify(who));
		assert.equal(who.body["workerCode"], "UN-1");
		const r = await a.refresh();
		assert.equal(r.deviceMode, "UNENROLLED");
		assert.equal(r.deviceId, null);
		assert.equal((await whoami(a, sc)).status, 200);
		await assert.rejects(a.logout({ scope: "device-all" }), code("INVALID_REQUEST"));
		assert.ok(a.session, "the refused device-all left the session alone");
		await a.logout();
		assert.equal(a.session, null);
	});

	scenarioTest("unenrolled login: no client identifier is INVALID_REQUEST; unknown client is INVALID_CREDENTIALS", async (sc) => {
		await op(sc, { op: "worker", code: "un-2", pin: PIN });
		const a = mkAuth(sc);
		await assert.rejects(a.login({ workerCode: "un-2", pin: PIN }), code("INVALID_REQUEST"));
		await assert.rejects(a.login({ workerCode: "un-2", pin: PIN, clientIdentifier: "no-such-client-xyz" }), code("INVALID_CREDENTIALS"));
		// the same code as a wrong PIN
		await assert.rejects(a.login({ workerCode: "un-2", pin: "000000", clientIdentifier: sc.clientIdentifier }), code("INVALID_CREDENTIALS"));
		assert.equal(a.session, null);
	});

	scenarioTest("requireEnrolledDevice refuses an unenrolled key with UNKNOWN_DEVICE", async (sc) => {
		await op(sc, { op: "worker", code: "un-3", pin: PIN });
		await op(sc, { op: "policy", column: "require_enrolled_device", value: true });
		const a = mkAuth(sc, { clientIdentifier: sc.clientIdentifier });
		await assert.rejects(a.login({ workerCode: "un-3", pin: PIN }), code("UNKNOWN_DEVICE"));
		// an enrolled device is unaffected
		const d = await enrolled(sc);
		const s = await d.login({ workerCode: "un-3", pin: PIN });
		assert.equal(s.deviceMode, "PERSONAL");
		assert.match(s.deviceId ?? "", /^fwd_/);
	});

	scenarioTest("an enrolled device naming another client gets CLIENT_MISMATCH", async (sc) => {
		await op(sc, { op: "worker", code: "cm-1", pin: PIN });
		const other = await newScenario();
		const d = await enrolled(sc);
		await assert.rejects(d.login({ workerCode: "cm-1", pin: PIN, clientIdentifier: other.clientIdentifier }), code("CLIENT_MISMATCH"));
		const ok = await d.login({ workerCode: "cm-1", pin: PIN, clientIdentifier: sc.clientIdentifier });
		assert.equal(ok.deviceMode, "PERSONAL");
	});

	scenarioTest("badge: badge + PIN, badge alone needs a PIN, normalisation, revocation", async (sc) => {
		const w = await op(sc, { op: "worker", code: "bg-1", pin: PIN });
		const { token } = await op(sc, { op: "badge", id: w["id"] });
		const d = await enrolled(sc);
		const un = mkAuth(sc);

		await assert.rejects(d.login({ badgeToken: token as string }), code("PIN_REQUIRED"));
		await assert.rejects(un.login({ badgeToken: token as string }), code("PIN_REQUIRED"));

		const s = await d.login({ badgeToken: token as string, pin: PIN });
		assert.equal(s.worker.workerCode, "BG-1");
		assert.equal(s.deviceMode, "PERSONAL");
		// amr is on the token; read the stored access token through the session's refresh store
		const amr = amrOf(await accessTokenOf(d));
		assert.deepEqual(amr, ["badge", "pin"]);

		// lower case and whitespace padding are the same badge, and an
		// unenrolled key needs no client identifier (the badge carries it)
		const messy = `  ${(token as string).toLowerCase()}\n`;
		const u = await un.login({ badgeToken: messy, pin: PIN });
		assert.equal(u.deviceMode, "UNENROLLED");
		assert.equal(u.deviceId, null);

		// a reissue revokes the first badge
		const { token: second } = await op(sc, { op: "badge", id: w["id"] });
		await assert.rejects(d.login({ badgeToken: token as string, pin: PIN }), code("INVALID_CREDENTIALS"));
		const again = await d.login({ badgeToken: second as string, pin: PIN });
		assert.equal(again.worker.workerCode, "BG-1");
	});

	scenarioTest("badgeOnlyLogin: enrolled device works with the badge alone, unenrolled still needs the PIN", async (sc) => {
		const w = await op(sc, { op: "worker", code: "bo-1", pin: PIN });
		const { token } = await op(sc, { op: "badge", id: w["id"] });
		await op(sc, { op: "policy", column: "badge_only_login", value: true });
		const d = await enrolled(sc);
		const un = mkAuth(sc);
		const s = await d.login({ badgeToken: token as string });
		assert.equal(s.worker.workerCode, "BO-1");
		assert.deepEqual(amrOf(await accessTokenOf(d)), ["badge"]);
		await assert.rejects(un.login({ badgeToken: token as string }), code("PIN_REQUIRED"));
	});

	scenarioTest("supervisor actAs: actor, endImpersonation, nesting, forbidden, wrong PIN, changePin", async (sc) => {
		const sup = await op(sc, { op: "supervisor", code: "sup-1", pin: PIN });
		await op(sc, { op: "worker", code: "tgt-1", pin: PIN2 });
		await op(sc, { op: "worker", code: "plain-1", pin: "351842" });
		const a = await enrolled(sc);
		const own = await a.login({ workerCode: "sup-1", pin: PIN });
		assert.equal(own.actor, null);

		await assert.rejects(a.actAs({ workerCode: "tgt-1", pin: "000000" }), code("INVALID_CREDENTIALS"));
		assert.equal(a.session?.worker.workerCode, "SUP-1", "a failed act-as leaves the supervisor signed in");

		const imp = await a.actAs({ workerCode: "tgt-1", pin: PIN });
		assert.equal(imp.worker.workerCode, "TGT-1");
		assert.equal(imp.actor?.id, sup["id"]);
		assert.equal(imp.actor?.type, "field_worker");
		assert.equal(imp.pinChangeRequired, false);
		assert.deepEqual(amrOf(await accessTokenOf(a)), ["pin", "imp"]);
		const who = await whoami(a, sc);
		assert.equal(who.status, 200, JSON.stringify(who));
		assert.equal(who.body["workerCode"], "TGT-1");
		assert.deepEqual(who.body["actor"], { id: sup["id"], type: "field_worker" });

		await assert.rejects(a.actAs({ workerCode: "plain-1", pin: PIN }), code("IMPERSONATING"));
		await assert.rejects(a.changePin({ currentPin: PIN, newPin: "628413" }), code("IMPERSONATING"));

		const back = await a.endImpersonation();
		assert.equal(back?.worker.workerCode, "SUP-1");
		assert.equal(back?.actor, null);
		const after = await whoami(a, sc);
		assert.equal(after.status, 200, JSON.stringify(after));
		assert.equal(after.body["workerCode"], "SUP-1");
		await assert.rejects(a.endImpersonation(), code("NOT_IMPERSONATING"));
	});

	scenarioTest("actAs by a non-supervisor is IMPERSONATION_FORBIDDEN", async (sc) => {
		await op(sc, { op: "worker", code: "plain-2", pin: PIN });
		await op(sc, { op: "worker", code: "tgt-2", pin: PIN2 });
		const a = await enrolled(sc);
		await a.login({ workerCode: "plain-2", pin: PIN });
		await assert.rejects(a.actAs({ workerCode: "tgt-2", pin: PIN }), code("IMPERSONATION_FORBIDDEN"));
		assert.equal(a.session?.worker.workerCode, "PLAIN-2");
	});

	scenarioTest("suspending the supervisor ends the impersonated session on refresh and falls back", async (sc) => {
		const sup = await op(sc, { op: "supervisor", code: "sup-3", pin: PIN });
		await op(sc, { op: "worker", code: "tgt-3", pin: PIN2 });
		const a = await enrolled(sc);
		await a.login({ workerCode: "sup-3", pin: PIN });
		const imp = await a.actAs({ workerCode: "tgt-3", pin: PIN });
		assert.equal(imp.actor?.id, sup["id"]);
		await op(sc, { op: "suspend", id: sup["id"] });
		await assert.rejects(a.refresh(), (e: unknown) => e instanceof FieldAuthError);
		// the library handed back to the supervisor's kept-aside session; no actor now
		assert.equal(a.session?.actor ?? null, null);
		assert.equal(a.session?.worker.workerCode, "SUP-3");
	});

	scenarioTest("sign-in code: principal actor, second redeem INVALID_CODE", async (sc) => {
		const w = await op(sc, { op: "worker", code: "sc-1", pin: PIN });
		const { code: c } = await op(sc, { op: "signInCode", id: w["id"] });
		const a = mkAuth(sc);
		const s = await a.redeemSignInCode(c as string);
		assert.equal(s.worker.workerCode, "SC-1");
		assert.equal(s.actor?.type, "principal");
		assert.equal(s.test, false);
		const who = await whoami(a, sc);
		assert.equal(who.status, 200, JSON.stringify(who));
		assert.equal((who.body["actor"] as { type: string }).type, "principal");
		await assert.rejects(mkAuth(sc).redeemSignInCode(c as string), code("INVALID_CODE"));
		// no supervisor behind it: ending the impersonation just ends the session
		assert.equal(await a.endImpersonation(), null);
		assert.equal(a.session, null);
	});

	scenarioTest("a test identity's session is flagged test", async (sc) => {
		const w = await op(sc, { op: "worker", code: "ti-1", pin: PIN });
		await op(sc, { op: "testIdentity", id: w["id"] });
		await op(sc, { op: "worker", code: "ti-real", pin: PIN });
		const d = await enrolled(sc);
		assert.equal((await d.login({ workerCode: "ti-real", pin: PIN })).test, false);
		const s = await d.login({ workerCode: "ti-1", pin: PIN });
		assert.equal(s.test, true);
		const who = await whoami(d, sc);
		assert.equal(who.body["test"], true);
		// a sign-in code for a test identity
		const { code: c } = await op(sc, { op: "signInCode", id: w["id"] });
		const viaCode = await mkAuth(sc).redeemSignInCode(c as string);
		assert.equal(viaCode.test, true);
	});
});
