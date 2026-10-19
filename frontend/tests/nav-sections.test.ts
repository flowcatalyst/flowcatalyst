import { describe, expect, it } from "vitest";
import {
	NAV_CLOSED_KEY,
	isSectionOpen,
	loadClosedSections,
	saveClosedSections,
	sectionHasActive,
	toggleSection,
} from "../src/utils/nav-sections";

const sec = {
	label: "Messaging",
	items: [
		{ route: "/events" },
		{ children: [{ route: "/platform/settings/theme" }] },
	],
};
const other = { label: "Platform", items: [{ route: "/clients" }] };

describe("nav sections", () => {
	it("detects the active section, including children and sub-paths", () => {
		expect(sectionHasActive(sec, "/events")).toBe(true);
		expect(sectionHasActive(sec, "/events/abc")).toBe(true);
		expect(sectionHasActive(sec, "/platform/settings/theme")).toBe(true);
		expect(sectionHasActive(sec, "/eventsX")).toBe(false);
	});

	it("keeps the active section open even when stored as closed", () => {
		const closed = new Set(["Messaging", "Platform"]);
		expect(isSectionOpen(sec, closed, "/events")).toBe(true);
		expect(isSectionOpen(other, closed, "/events")).toBe(false);
		expect(isSectionOpen(other, new Set(), "/events")).toBe(true);
	});

	it("toggles other sections but not the active one", () => {
		const c1 = toggleSection(other, new Set(), "/events");
		expect(c1.has("Platform")).toBe(true);
		expect(toggleSection(other, c1, "/events").has("Platform")).toBe(false);
		expect(toggleSection(sec, new Set(), "/events").size).toBe(0);
	});

	it("round-trips through storage and survives bad data / no storage", () => {
		const mem = new Map<string, string>();
		const st = {
			getItem: (k: string) => mem.get(k) ?? null,
			setItem: (k: string, v: string) => void mem.set(k, v),
		};
		saveClosedSections(new Set(["B", "A"]), st);
		expect(mem.get(NAV_CLOSED_KEY)).toBe('["A","B"]');
		expect([...loadClosedSections(st)]).toEqual(["A", "B"]);

		mem.set(NAV_CLOSED_KEY, "{not json");
		expect(loadClosedSections(st).size).toBe(0);
		mem.set(NAV_CLOSED_KEY, '{"a":1}');
		expect(loadClosedSections(st).size).toBe(0);
		mem.set(NAV_CLOSED_KEY, '["x",3]');
		expect([...loadClosedSections(st)]).toEqual(["x"]);

		const throwing = {
			getItem: () => {
				throw new Error("blocked");
			},
			setItem: () => {
				throw new Error("blocked");
			},
		};
		expect(loadClosedSections(throwing).size).toBe(0);
		expect(() => saveClosedSections(new Set(["A"]), throwing)).not.toThrow();
	});
});
