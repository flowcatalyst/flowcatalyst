/**
 * The post-login return to the server-rendered UI (src/api/uiRedirect.ts):
 * only a same-origin path under /ui is followed; everything else falls back
 * to the normal landing page (no open redirect).
 */

import { describe, expect, it } from "vitest";
import { safeUiRedirect } from "@/api/uiRedirect";

const origin = "https://fc.example.com";

describe("safeUiRedirect", () => {
	it("accepts /ui paths with query and hash", () => {
		expect(safeUiRedirect("/ui", origin)).toBe("/ui");
		expect(safeUiRedirect("/ui/", origin)).toBe("/ui/");
		expect(safeUiRedirect("/ui/dispatch-pools?q=a%26b&status=ACTIVE", origin)).toBe(
			"/ui/dispatch-pools?q=a%26b&status=ACTIVE",
		);
		expect(safeUiRedirect("/ui/x#top", origin)).toBe("/ui/x#top");
	});

	it("rejects missing values and non-/ui paths", () => {
		expect(safeUiRedirect(null, origin)).toBeNull();
		expect(safeUiRedirect("", origin)).toBeNull();
		expect(safeUiRedirect("/dashboard", origin)).toBeNull();
		expect(safeUiRedirect("/uix", origin)).toBeNull();
		expect(safeUiRedirect("/ui-evil", origin)).toBeNull();
		expect(safeUiRedirect("ui/x", origin)).toBeNull();
	});

	it("rejects other origins however they are spelled", () => {
		for (const raw of [
			"https://evil.example/ui/x",
			"//evil.example/ui/x",
			"/\\evil.example/ui/x",
			"\\\\evil.example/ui/x",
			"/\t/evil.example/ui/x",
			"javascript:alert(1)//ui/",
			"https://fc.example.com.evil.example/ui/x",
		]) {
			expect(safeUiRedirect(raw, origin), raw).toBeNull();
		}
	});

	it("rejects paths that normalise out of /ui", () => {
		expect(safeUiRedirect("/ui/../dashboard", origin)).toBeNull();
		expect(safeUiRedirect("/ui/%2e%2e/dashboard", origin)).toBeNull();
		expect(safeUiRedirect("/ui/./../../oauth/authorize", origin)).toBeNull();
	});
});
