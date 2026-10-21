/**
 * Function Diagnostics (trace logs): countdown, filter-to-query mapping, body
 * display / redaction and permission gating. Pure logic lives in
 * src/pages/functions/functionTrace.ts; the wiring is pinned against the SFC
 * source like the neighbouring page tests.
 */

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { userHasPermission } from "@/stores/permissions";
import {
	BODIES_HIDDEN_MESSAGE,
	FUNCTION_TRACE_MANAGE,
	FUNCTION_TRACE_VIEW,
	FUNCTION_TRACE_VIEW_BODIES,
	anyLive,
	bodyView,
	buildTraceQuery,
	countdown,
	durationChoices,
	emptyFilters,
	enableBlockedReason,
	hasActiveFilters,
	headerRows,
	httpSummary,
	invocationCounts,
	isValidSampleRate,
	parseHttpData,
	traceGates,
} from "@/pages/functions/functionTrace";

const src = (rel: string) =>
	readFileSync(fileURLToPath(new URL(`../src/pages/functions/${rel}`, import.meta.url)), "utf8");

const NOW = Date.parse("2026-10-21T10:00:00Z");

describe("countdown", () => {
	it("is live with an m:ss label until enabledUntil", () => {
		const c = countdown("2026-10-21T10:04:05Z", NOW);
		expect(c.live).toBe(true);
		expect(c.label).toBe("4:05");
	});

	it("shows hours from an hour up and rounds partial seconds up", () => {
		expect(countdown("2026-10-21T11:00:00Z", NOW).label).toBe("1:00:00");
		expect(countdown("2026-10-21T10:00:00.400Z", NOW).label).toBe("0:01");
	});

	it("flips to off the moment the window passes", () => {
		expect(countdown("2026-10-21T10:00:00Z", NOW).live).toBe(false);
		expect(countdown("2026-10-21T09:59:59Z", NOW).live).toBe(false);
		expect(countdown("2026-10-21T10:00:01Z", NOW).live).toBe(true);
		expect(countdown("2026-10-21T10:00:01Z", NOW + 1000).live).toBe(false);
	});

	it("treats null and garbage as off", () => {
		expect(countdown(null, NOW)).toEqual({ live: false, remainingMs: 0, label: "" });
		expect(countdown("nope", NOW).live).toBe(false);
	});

	it("anyLive drives auto refresh", () => {
		const ctx = (enabledUntil: string | null) =>
			({ name: "x", description: "", builtIn: false, enabledUntil, sampleRate: 1, live: false, maxMinutes: 60 });
		expect(anyLive([ctx(null), ctx("2026-10-21T09:00:00Z")], NOW)).toBe(false);
		expect(anyLive([ctx(null), ctx("2026-10-21T10:01:00Z")], NOW)).toBe(true);
	});
});

describe("enable form limits", () => {
	it("offers 5/15/30/60 within the context's cap", () => {
		expect(durationChoices(60)).toEqual([5, 15, 30, 60]);
		expect(durationChoices(15)).toEqual([5, 15]);
		expect(durationChoices(3)).toEqual([3]);
	});

	it("accepts only whole sample rates 1..1000", () => {
		expect(isValidSampleRate(1)).toBe(true);
		expect(isValidSampleRate(1000)).toBe(true);
		expect(isValidSampleRate(0)).toBe(false);
		expect(isValidSampleRate(1001)).toBe(false);
		expect(isValidSampleRate(2.5)).toBe(false);
		expect(isValidSampleRate(null)).toBe(false);
	});
});

describe("filter to query", () => {
	it("sends only what is set", () => {
		expect(buildTraceQuery(emptyFilters(), NOW)).toEqual({ limit: 100 });
	});

	it("maps every filter, trims text and computes since from the quick choice", () => {
		const q = buildTraceQuery(
			{
				context: "http",
				kind: "log",
				level: "error",
				reference: " ORD-1 ",
				correlationId: "c-9",
				invocationId: "inv-7",
				sinceMinutes: 60,
			},
			NOW,
			{ cursor: "abc", limit: 50 },
		);
		expect(q).toEqual({
			limit: 50,
			context: "http",
			kind: "log",
			level: "error",
			reference: "ORD-1",
			correlationId: "c-9",
			invocationId: "inv-7",
			since: "2026-10-21T09:00:00.000Z",
			cursor: "abc",
		});
	});

	it("knows when filters are active", () => {
		expect(hasActiveFilters(emptyFilters())).toBe(false);
		expect(hasActiveFilters({ ...emptyFilters(), level: "warn" })).toBe(true);
	});

	it("counts records per invocation", () => {
		const r = (invocationId: string) => ({ invocationId }) as never;
		const counts = invocationCounts([r("a"), r("b"), r("a")]);
		expect(counts.get("a")).toBe(2);
		expect(counts.get("b")).toBe(1);
	});
});

describe("body display", () => {
	it("pretty-prints valid JSON", () => {
		const v = bodyView({ body: '{"a":1}', bodyEncoding: "utf8" }, false);
		expect(v).toMatchObject({ state: "shown", label: "JSON", text: '{\n  "a": 1\n}' });
	});

	it("shows non-JSON text as text and labels base64", () => {
		expect(bodyView({ body: "hello", bodyEncoding: "utf8" }, false)).toMatchObject({ label: "text", text: "hello" });
		expect(bodyView({ body: "aGk=", bodyEncoding: "base64" }, false)).toMatchObject({ label: "base64", text: "aGk=" });
	});

	it("notes truncation with the total size and does not try to parse it", () => {
		const v = bodyView({ body: '{"a":', bodyEncoding: "utf8", bodyTruncated: true, bodyBytes: 70000 }, false);
		expect(v.label).toBe("text");
		expect(v.note).toBe(`truncated (${(70000).toLocaleString()} bytes total)`);
	});

	it("hides bodies when the record is redacted, whatever it carries", () => {
		const v = bodyView({ body: null, bodyEncoding: "utf8", bodyBytes: 12 }, true);
		expect(v).toEqual({ state: "hidden", text: "", label: "", note: "" });
		expect(bodyView({ body: "secret" }, true).text).toBe("");
	});

	it("reports an empty body", () => {
		expect(bodyView({ body: null }, false).state).toBe("empty");
	});

	it("explains redaction in the UI", () => {
		expect(BODIES_HIDDEN_MESSAGE).toBe(
			"Bodies hidden: you do not have permission to view request and response bodies",
		);
		const sfc = src("FunctionDiagnostics.vue");
		expect(sfc).toMatch(/v-if="data\.bodiesRedacted"[^>]*>\s*\{\{ BODIES_HIDDEN_MESSAGE \}\}/);
		expect(sfc).toContain("bodyView(d.request, redacted)");
		expect(sfc).toContain("r.bodiesRedacted === true");
	});
});

describe("http record data", () => {
	const data = {
		request: { method: "POST", path: "/orders", rawQuery: "a=1", headers: { "X-B": ["1", "2"], "A": "z" }, body: null },
		response: { status: 201, headers: {}, body: null },
		durationMs: 12,
		outcome: "ok",
	};

	it("parses the shape and rejects others", () => {
		expect(parseHttpData(data)).not.toBeNull();
		expect(parseHttpData(null)).toBeNull();
		expect(parseHttpData({ request: {} })).toBeNull();
	});

	it("summarises and sorts headers", () => {
		expect(httpSummary(parseHttpData(data)!)).toBe("POST /orders?a=1 → 201 · 12 ms · ok");
		expect(headerRows(data.request.headers)).toEqual([
			["A", "z"],
			["X-B", "1, 2"],
		]);
	});
});

describe("permission gating", () => {
	it("codes are the API's", () => {
		expect(FUNCTION_TRACE_VIEW).toBe("platform:function:trace:view");
		expect(FUNCTION_TRACE_MANAGE).toBe("platform:function:trace:manage");
		expect(FUNCTION_TRACE_VIEW_BODIES).toBe("platform:function:trace:view-bodies");
	});

	const gatesFor = (permissions: string[]) => traceGates((p) => userHasPermission({ permissions }, p));

	it("view alone cannot change tracing", () => {
		const g = gatesFor([FUNCTION_TRACE_VIEW]);
		expect(g).toEqual({ view: true, manage: false, viewBodies: false });
		expect(enableBlockedReason("orders", g)).not.toBeNull();
	});

	it("manage enables declared contexts, but http needs view-bodies too", () => {
		const g = gatesFor([FUNCTION_TRACE_VIEW, FUNCTION_TRACE_MANAGE]);
		expect(enableBlockedReason("orders", g)).toBeNull();
		expect(enableBlockedReason("http", g)).toMatch(/view-bodies/);
		const all = gatesFor([FUNCTION_TRACE_VIEW, FUNCTION_TRACE_MANAGE, FUNCTION_TRACE_VIEW_BODIES]);
		expect(enableBlockedReason("http", all)).toBeNull();
	});

	it("the SFC hides controls without manage and shows the section in the drawer", () => {
		const sfc = src("FunctionDiagnostics.vue");
		expect(sfc).toMatch(/<Column v-if="gates\.manage" header="Actions"/);
		expect(sfc).toContain(':disabled="enableBlockedReason(data.name, gates) !== null"');
		expect(sfc).toMatch(/<Message v-if="!gates\.view"/);
		expect(src("FunctionDetailDrawer.vue")).toContain("<FunctionDiagnostics :function-id=\"fn.id\" />");
	});
});
