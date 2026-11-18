import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

// Every platform call a hand-written wrapper makes must be an operation of the
// spec this package carries (openapi/openapi.json, the generator's input, and
// its app and admin splits, which add the SDK-only routes). A wrapper that
// calls a path or method the platform does not have can only ever fail
// (getServiceAccount called GET /api/applications/{id}/service-account before
// the platform had it; updateClientConfig a PUT that never existed). The
// check reads the source, so a new wrapper is covered without a test of its
// own.

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
type Spec = { paths: Record<string, Record<string, { operationId?: string }>> };
const specs = ["openapi.json", "openapi.app.json", "openapi.admin.json"].map(
	(name) => JSON.parse(readFileSync(join(root, "openapi", name), "utf8")) as Spec,
);

const METHODS = ["get", "post", "put", "delete", "patch", "head"];

/** `${...}`, with one level of braces inside (`${qs({ dryRun })}`). */
const INTERPOLATION = String.raw`\$\{(?:[^{}]|\{[^{}]*\})*\}`;

/** `/api/a/{id}/b` and `/api/a/${enc(id)}/b` both become `/api/a/{}/b`. */
function normalise(path: string): string {
	return (
		path
			// a trailing interpolation glued to a segment is a query string
			.replace(new RegExp(`(?<=[^/])${INTERPOLATION}$`), "")
			.replace(new RegExp(INTERPOLATION, "g"), "{}")
			.replace(/\{[^}]*\}/g, "{}")
			.split("?")[0]!
			.replace(/\/$/, "")
	);
}

const operations = new Map<string, Set<string>>();
const operationIds = new Set<string>();
for (const [path, item] of specs.flatMap((spec) => Object.entries(spec.paths))) {
	for (const [method, op] of Object.entries(item)) {
		if (!METHODS.includes(method)) continue;
		const key = normalise(path);
		if (!operations.has(key)) operations.set(key, new Set());
		operations.get(key)!.add(method.toUpperCase());
		if (op.operationId) operationIds.add(op.operationId);
	}
}

function sourceFiles(dir: string): string[] {
	return readdirSync(dir).flatMap((name) => {
		const full = join(dir, name);
		if (statSync(full).isDirectory()) return name === "generated" ? [] : sourceFiles(full);
		return name.endsWith(".ts") ? [full] : [];
	});
}

type Call = { where: string; method: string; path: string };

function lineOf(text: string, index: number): number {
	return text.slice(0, index).split("\n").length;
}

/** The calls a file makes, with constants (`const PATH = "/api/..."`) resolved. */
function callsIn(file: string): { calls: Call[]; sdkOps: { where: string; op: string }[] } {
	const text = readFileSync(file, "utf8");
	const rel = relative(root, file);
	const consts = new Map<string, string>();
	for (const m of text.matchAll(/const ([A-Z_]+) = "([^"]+)";/g)) consts.set(m[1]!, m[2]!);
	const resolve = (expr: string): string | undefined => {
		const lit = /^["'`](.*)["'`]$/s.exec(expr.trim());
		const raw = lit ? lit[1]! : consts.get(expr.trim());
		if (raw === undefined) return undefined;
		return raw.replace(/\$\{([A-Z_]+)\}/g, (all, name: string) => consts.get(name) ?? all);
	};
	const calls: Call[] = [];
	// httpClient.get({ url: "/api/..." })
	for (const m of text.matchAll(/\b\w+\.(get|post|put|delete|patch)\(\{\s*url:\s*(["'`][^"'`]+["'`])/g)) {
		calls.push({ where: `${rel}:${lineOf(text, m.index!)}`, method: m[1]!.toUpperCase(), path: resolve(m[2]!)! });
	}
	// this.json("GET", `${PATH}/...`) / this.fetch<T>("POST", PATH, body)
	for (const m of text.matchAll(/this\.(?:json|fetch)(?:<[^>(]*>)?\(\s*"(GET|POST|PUT|DELETE|PATCH)",\s*([A-Z_]+|"[^"]*"|`[^`]*`)/g)) {
		const path = resolve(m[2]!);
		assert.ok(path, `${rel}:${lineOf(text, m.index!)}: cannot resolve ${m[2]}`);
		calls.push({ where: `${rel}:${lineOf(text, m.index!)}`, method: m[1]!, path });
	}
	// The definition synchronizer posts `/api/applications/{app}/${resource}/sync`
	// for each resource it names in `this.post(app, "<resource>", ...)`.
	if (text.includes("/${resource}/sync")) {
		const resources = [...text.matchAll(/this\.post\(\s*[\w.]+,\s*"([a-z-]+)"/gs)].map((m) => m[1]!);
		assert.ok(resources.length > 0, `${rel}: no synchronizer resources found`);
		for (let i = calls.length - 1; i >= 0; i--) {
			if (calls[i]!.path.includes("${resource}")) {
				const [call] = calls.splice(i, 1);
				for (const r of new Set(resources)) {
					calls.push({ ...call!, path: call!.path.replace("${resource}", r) });
				}
			}
		}
	}
	const sdkOps = [...text.matchAll(/\bsdk\.(\w+)\(/g)].map((m) => ({
		where: `${rel}:${lineOf(text, m.index!)}`,
		op: m[1]!,
	}));
	return { calls, sdkOps };
}

test("every path a hand-written wrapper calls is in the spec, with that method", () => {
	const problems: string[] = [];
	let checked = 0;
	const seen = new Set<string>();
	for (const file of sourceFiles(join(root, "src"))) {
		const { calls, sdkOps } = callsIn(file);
		for (const c of calls) {
			if (!c.path.startsWith("/api/")) continue; // the router's /monitoring paths are not the platform's
			checked++;
			const key = normalise(c.path);
			seen.add(`${c.method} ${key}`);
			const methods = operations.get(key);
			if (!methods) problems.push(`${c.where}: ${c.method} ${c.path}: no such path`);
			else if (!methods.has(c.method))
				problems.push(`${c.where}: ${c.method} ${c.path}: the path has only ${[...methods].join(", ")}`);
		}
		for (const s of sdkOps) {
			checked++;
			if (!operationIds.has(s.op)) problems.push(`${s.where}: sdk.${s.op}: no such operation`);
		}
	}
	assert.deepEqual(problems, []);
	// The scan must keep finding the calls (a regex that stops matching would
	// pass vacuously).
	assert.ok(checked > 150, `only ${checked} calls found`);
	for (const known of [
		"GET /api/applications/{}",
		"POST /api/applications/{}/provision-service-account",
		"GET /api/scheduled-jobs/by-code/{}",
		"PUT /api/ingestion-sets/{}",
		"POST /api/applications/{}/subscriptions/sync",
	]) {
		assert.ok(seen.has(known), `the scan no longer sees ${known}`);
	}
});
