import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";

// ios/FieldIos is a verbatim copy of clients/field-ios (Sources/FlowCatalystFieldCore -> Core,
// Sources/FlowCatalystField -> Apple, VERSION), made by scripts/sync-field-ios.mjs: the pod compiles it together
// with the bridge. In this monorepo the copy must equal its source; the published, split-out repository has no
// ../field-ios, so the check is skipped there.
const here = dirname(fileURLToPath(import.meta.url));
const pkg = join(here, "..");
const upstream = join(pkg, "..", "field-ios");
const present = existsSync(upstream);

describe("ios/FieldIos", () => {
	it("is byte-identical to clients/field-ios", { skip: !present && "../field-ios is not next to this package" }, async () => {
		const { filePairs, strayFiles } = (await import(pathToFileURL(join(pkg, "scripts", "sync-field-ios.mjs")).href)) as {
			filePairs(): [string, string][];
			strayFiles(): string[];
		};
		const pairs = filePairs();
		assert.ok(pairs.length > 20, "the field-ios sources were not found");
		for (const [from, to] of pairs) {
			assert.ok(existsSync(to), `${relative(pkg, to)} is missing: run pnpm sync:field-ios`);
			assert.ok(readFileSync(from).equals(readFileSync(to)), `${relative(pkg, to)} differs from ${relative(pkg, from)}: run pnpm sync:field-ios`);
		}
		assert.deepEqual(strayFiles().map((f) => relative(pkg, f)), [], "files copied earlier no longer exist in field-ios: run pnpm sync:field-ios");
	});
});
