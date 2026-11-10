import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");

test("SubscriptionMode is exactly the platform's three dispatch modes", () => {
	// tsc exits non-zero (execFileSync throws) if the Record in
	// subscription-mode.check.ts is missing a member or has an extra key.
	try {
		execFileSync(
			join(root, "node_modules/.bin/tsc"),
			["-p", "tests/tsconfig.subscription-mode.json"],
			{ cwd: root, stdio: "pipe" },
		);
	} catch (e) {
		const out = (e as { stdout?: Buffer }).stdout?.toString() ?? String(e);
		assert.fail(`SubscriptionMode does not match the platform modes:\n${out}`);
	}
});
