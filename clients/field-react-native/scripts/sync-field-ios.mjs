// Copies the Swift sources of clients/field-ios into ios/FieldIos (the pod compiles them together with the bridge),
// and deletes copied files that no longer exist upstream.
//
//   node scripts/sync-field-ios.mjs            copy
//   node scripts/sync-field-ios.mjs --check    exit 1 when the copy differs from the source
//
// The published package is installed from a repository that holds only this directory, so the pod cannot reach
// ../field-ios; the copy is what ships. tests/field-ios-sync.test.ts checks it in the monorepo.
import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const pkg = join(here, "..");
const upstream = join(pkg, "..", "field-ios");

/** [source dir, destination dir] pairs, relative to the upstream package and to this package. */
export const MAPPINGS = [
	[join("Sources", "FlowCatalystFieldCore"), join("ios", "FieldIos", "Core")],
	[join("Sources", "FlowCatalystField"), join("ios", "FieldIos", "Apple")],
];
export const VERSION_FILE = [join("VERSION"), join("ios", "FieldIos", "VERSION")];

const swiftFiles = (dir) => (existsSync(dir) ? readdirSync(dir).filter((f) => f.endsWith(".swift")).sort() : []);

/** Every [source, destination] file pair the copy consists of. */
export function filePairs() {
	const pairs = [];
	for (const [src, dst] of MAPPINGS) {
		for (const f of swiftFiles(join(upstream, src))) pairs.push([join(upstream, src, f), join(pkg, dst, f)]);
	}
	pairs.push([join(upstream, VERSION_FILE[0]), join(pkg, VERSION_FILE[1])]);
	return pairs;
}

/** Copied files whose source is gone. */
export function strayFiles() {
	const stray = [];
	for (const [src, dst] of MAPPINGS) {
		const keep = new Set(swiftFiles(join(upstream, src)));
		for (const f of swiftFiles(join(pkg, dst))) if (!keep.has(f)) stray.push(join(pkg, dst, f));
	}
	return stray;
}

export function differences() {
	const out = [];
	for (const [from, to] of filePairs()) {
		if (!existsSync(to) || !readFileSync(from).equals(readFileSync(to))) out.push(to);
	}
	return [...out, ...strayFiles()];
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
	if (!existsSync(upstream)) {
		console.error(`sync-field-ios: ${upstream} not found (run this inside the monorepo)`);
		process.exit(2);
	}
	if (process.argv.includes("--check")) {
		const d = differences();
		if (d.length > 0) {
			console.error("sync-field-ios: out of date:\n" + d.map((f) => "  " + f).join("\n"));
			process.exit(1);
		}
		console.log("sync-field-ios: up to date");
	} else {
		for (const f of strayFiles()) rmSync(f);
		for (const [from, to] of filePairs()) {
			mkdirSync(dirname(to), { recursive: true });
			copyFileSync(from, to);
		}
		console.log(`sync-field-ios: copied ${filePairs().length} files`);
	}
}
