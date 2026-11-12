import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";

// push-core.ts is a verbatim copy of the push reader core, whose source is the framework's
// ui/packages/push/src/core.ts; field-js carries the same copy. In this monorepo both copies are
// present and must be identical; the published, split-out repo has no sibling, so the check is skipped.
const here = dirname(fileURLToPath(import.meta.url));
const mine = join(here, "..", "src", "push-core.ts");
const theirs = join(here, "..", "..", "field-js", "src", "push-core.ts");

describe("push-core.ts", () => {
	it("is byte-identical to field-js's copy", { skip: !existsSync(theirs) && "../field-js is not next to this package" }, () => {
		assert.ok(readFileSync(mine).equals(readFileSync(theirs)), "clients/field-react-native/src/push-core.ts differs from clients/field-js/src/push-core.ts: copy the framework's ui/packages/push/src/core.ts over both");
	});
});
