/**
 * A theme's logo SVG is stored text an administrator pasted. Inserted with
 * v-html, an SVG like <svg/onload=...> or <svg><image/href=x/onerror=...>
 * runs script on the sign-in pages and in the sidebar of every signed-in
 * user. Shown through <img src="data:image/svg+xml;base64,..."> it runs none.
 */

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { svgDataUri } from "@/utils/svg-data-uri";

const sites = [
	"../src/pages/auth/LoginPage.vue",
	"../src/pages/auth/ForgotPasswordPage.vue",
	"../src/pages/auth/ResetPasswordPage.vue",
	"../src/components/layout/AppSidebar.vue",
	"../src/components/theme/LoginThemeEditor.vue",
];

function source(rel: string): string {
	return readFileSync(fileURLToPath(new URL(rel, import.meta.url)), "utf8");
}

describe("logo SVGs are images, never markup", () => {
	for (const rel of sites) {
		it(`${rel} shows the logo SVG through <img>`, () => {
			const src = source(rel);
			expect(src).not.toMatch(/v-html="[^"]*logoSvg/);
			expect(src).toMatch(/:src="svgDataUri\([^)]*logoSvg\)"/);
		});
	}

	it("svgDataUri encodes the markup as base64, unicode included", () => {
		const svg = `<svg xmlns="http://www.w3.org/2000/svg"><text>Café ✓</text></svg>`;
		const uri = svgDataUri(svg);
		expect(uri.startsWith("data:image/svg+xml;base64,")).toBe(true);
		const bytes = Uint8Array.from(atob(uri.slice("data:image/svg+xml;base64,".length)), (c) => c.charCodeAt(0));
		expect(new TextDecoder().decode(bytes)).toBe(svg);
	});

	it("a hostile SVG becomes inert data, not attributes", () => {
		const uri = svgDataUri(`<svg/onload=alert(1)>`);
		expect(uri).not.toContain("onload");
		expect(uri).not.toContain("<");
	});
});
