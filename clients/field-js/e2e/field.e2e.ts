// End-to-end against a real platform. It compiles and spawns the Go helper
// TestFieldJSE2EServer (internal/platform/fieldauth/e2eserver_pg_test.go): the
// real /field/* routes on an embedded Postgres, plus an application endpoint
// that verifies requests with the Go SDK's FieldTokenVerifier. Skips when Go
// or the embedded Postgres is unavailable. Kills only the child it spawned.

import { strict as assert } from "node:assert";
import { spawn, spawnSync, type ChildProcessWithoutNullStreams } from "node:child_process";
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
});
