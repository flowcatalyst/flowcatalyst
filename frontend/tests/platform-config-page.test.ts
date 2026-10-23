import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { getRoutePermission } from "@/stores/permissions";

const page = readFileSync(
	resolve(__dirname, "../src/pages/platform/settings/PlatformConfigPage.vue"),
	"utf8",
);
const api = readFileSync(resolve(__dirname, "../src/api/config.ts"), "utf8");

describe("platform configuration page", () => {
	it("is guarded by the config view permission", () => {
		expect(getRoutePermission("/platform/settings/config")).toBe(
			"platform:admin:config:view",
		);
	});

	it("never prefills a secret when editing", () => {
		expect(page).toContain('row.valueType === "SECRET" ? "" : row.value');
	});

	it("only offers reveal to holders of the reveal permission, after a confirmation", () => {
		expect(page).toContain("platform:admin:config:reveal");
		expect(page).toContain("v-if=\"data.valueType === 'SECRET' && canReveal\"");
		expect(page).toContain("recorded in the audit log");
	});

	it("drops the plaintext when the reveal dialog closes", () => {
		expect(page).toMatch(/function closeReveal\(\) \{[^}]*revealed\.value = ""/s);
	});

	it("reveals through the dedicated /secret endpoint with encoded segments", () => {
		expect(api).toContain("encodeURIComponent(section)");
		expect(api).toContain("/secret");
	});
});
