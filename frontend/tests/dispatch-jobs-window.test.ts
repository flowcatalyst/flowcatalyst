import { describe, expect, it } from "vitest";
import { windowFromHeaders } from "@/api/dispatch-jobs";
import {
	WINDOW_PRESETS,
	customRangeTooLarge,
	describeWindow,
	normalizePreset,
	selectCustom,
	selectPreset,
	windowBounds,
} from "@/pages/dispatch-jobs/jobWindow";

const now = new Date("2026-10-19T12:00:00.000Z");
const H = 3_600_000;

describe("presets", () => {
	it("are 1h, 24h (default), 3d, 7d, 31d", () => {
		expect(WINDOW_PRESETS.map((p) => p.key)).toEqual(["1h", "24h", "3d", "7d", "31d"]);
		expect(WINDOW_PRESETS.at(-1)!.ms).toBe(31 * 24 * H);
	});
	it("unknown or empty is the default", () => {
		expect(normalizePreset("")).toBe("24h");
		expect(normalizePreset("bogus")).toBe("24h");
		expect(normalizePreset("7d")).toBe("7d");
	});
	it("choosing a preset drops custom dates, custom dates win over the preset", () => {
		expect(selectPreset("3d")).toEqual({ preset: "3d", from: "", to: "" });
		expect(selectCustom("2026-10-01", "")).toMatchObject({ from: "2026-10-01" });
	});
});

describe("windowBounds", () => {
	it("sends nothing for the default so the server defaults (and says so)", () => {
		expect(windowBounds(selectPreset("24h"), now)).toEqual({});
	});
	it("sends since = now - span for the other presets", () => {
		expect(windowBounds(selectPreset("1h"), now)).toEqual({ since: new Date(now.getTime() - H).toISOString() });
		expect(windowBounds(selectPreset("7d"), now).since).toBe(new Date(now.getTime() - 7 * 24 * H).toISOString());
	});
	it("custom days: start of from, end of to", () => {
		const b = windowBounds(selectCustom("2026-10-01", "2026-10-03"), now);
		expect(b.since).toBe(new Date(2026, 9, 1).toISOString());
		expect(b.until).toBe(new Date(2026, 9, 4).toISOString());
	});
	it("an until cap only lowers until", () => {
		const cap = new Date(now.getTime() - 5 * 60_000);
		expect(windowBounds(selectPreset("24h"), now, cap)).toEqual({ until: cap.toISOString() });
		const earlier = windowBounds(selectCustom("2026-10-01", "2026-10-02"), now, cap);
		expect(earlier.until).toBe(new Date(2026, 9, 3).toISOString());
	});
	it("flags a custom range wider than the server's 31 days", () => {
		expect(customRangeTooLarge({ from: "2026-09-01", to: "2026-10-02" })).toBe(true);
		expect(customRangeTooLarge({ from: "2026-10-01", to: "2026-10-31" })).toBe(false);
		expect(customRangeTooLarge({ from: "2026-10-01", to: "" })).toBe(false);
	});
});

describe("describeWindow", () => {
	const fmt = (d: Date) => d.toISOString();
	it("says 'the last 24 hours' for a window ending now, and whether it was defaulted", () => {
		const w = { from: new Date(now.getTime() - 24 * H).toISOString(), to: now.toISOString(), defaulted: true };
		expect(describeWindow(w, now)).toEqual({ text: "Showing the last 24 hours", defaulted: true });
	});
	it("names hours, an hour and days", () => {
		const mk = (ms: number) => ({ from: new Date(now.getTime() - ms).toISOString(), to: now.toISOString(), defaulted: false });
		expect(describeWindow(mk(H), now).text).toBe("Showing the last hour");
		expect(describeWindow(mk(7 * 24 * H), now).text).toBe("Showing the last 7 days");
		expect(describeWindow(mk(31 * 24 * H), now).text).toBe("Showing the last 31 days");
	});
	it("shows the dates when the window ended earlier", () => {
		const w = { from: "2026-10-01T00:00:00.000Z", to: "2026-10-02T00:00:00.000Z", defaulted: false };
		expect(describeWindow(w, now, fmt).text).toBe("Showing 2026-10-01T00:00:00.000Z to 2026-10-02T00:00:00.000Z");
	});
});

describe("windowFromHeaders", () => {
	it("reads the echoed window", () => {
		const h = new Headers({ "X-Window-From": "a", "X-Window-To": "b", "X-Window-Defaulted": "true" });
		expect(windowFromHeaders(h)).toEqual({ from: "a", to: "b", defaulted: true });
		expect(windowFromHeaders(new Headers({ "X-Window-From": "a", "X-Window-To": "b" }))?.defaulted).toBe(false);
	});
	it("is null when the headers are absent", () => {
		expect(windowFromHeaders(new Headers())).toBeNull();
	});
});
