import { describe, expect, it } from "vitest";
import { buildQuery } from "@/api/dispatch-jobs";
import {
	EMPTY_FILTERS,
	MAX_EVENT_TYPES,
	activeLens,
	cleanEventTypes,
	explainListError,
	filtersToParams,
	lensFilters,
	suggestEventTypes,
	validateFilters,
} from "@/pages/dispatch-jobs/jobFilters";
import { summarizeFilters } from "@/pages/dispatch-jobs/requeueByFilter";

const now = new Date("2026-10-19T12:00:00.000Z");
const opts = { size: 200, sort: "createdAt.desc" as const, now };

describe("filtersToParams", () => {
	it("an untouched page sends no window (the server defaults to 24h)", () => {
		const p = filtersToParams(EMPTY_FILTERS, opts);
		expect(p.since).toBeUndefined();
		expect(p.until).toBeUndefined();
		expect(buildQuery(p)).toBe("?size=200&sort=createdAt.desc");
	});
	it("maps event types, subscription (as name) and connection (as code)", () => {
		const p = filtersToParams(
			{ ...EMPTY_FILTERS, eventTypes: [" a:b:c:d ", "a:b:c:d", "x:y:z:w"], subscription: " orders* ", connection: "billing" },
			opts,
		);
		expect(p.eventTypes).toEqual(["a:b:c:d", "x:y:z:w"]);
		expect(p.subscriptionName).toBe("orders*");
		expect(p.connectionCode).toBe("billing");
		const q = new URLSearchParams(buildQuery(p));
		expect(q.get("eventTypes")).toBe("a:b:c:d,x:y:z:w");
		expect(q.get("subscriptionName")).toBe("orders*");
		expect(q.get("connectionCode")).toBe("billing");
	});
	it("combines with the status filter and a preset window", () => {
		const p = filtersToParams({ ...EMPTY_FILTERS, statuses: ["FAILED"], window: "7d" }, opts);
		expect(p.statuses).toEqual(["FAILED"]);
		expect(p.since).toBe(new Date(now.getTime() - 7 * 24 * 3_600_000).toISOString());
	});
	it("the Stuck lens caps until at five minutes ago", () => {
		const p = filtersToParams({ ...EMPTY_FILTERS, ...lensFilters("stuck") }, opts);
		expect(p.statuses).toEqual(["PENDING", "QUEUED"]);
		expect(p.until).toBe(new Date(now.getTime() - 5 * 60_000).toISOString());
	});
	it("a stale stuck marker without the stuck statuses adds no cap", () => {
		const p = filtersToParams({ ...EMPTY_FILTERS, statuses: ["PENDING"], lens: "stuck" }, opts);
		expect(p.until).toBeUndefined();
	});
});

describe("lenses", () => {
	it("Failed is FAILED + ERROR", () => {
		expect(lensFilters("failed")).toEqual({ statuses: ["FAILED", "ERROR"], lens: "" });
		expect(activeLens({ statuses: ["ERROR", "FAILED"], lens: "" })).toBe("failed");
	});
	it("Stuck needs its marker", () => {
		expect(activeLens({ statuses: ["PENDING", "QUEUED"], lens: "stuck" })).toBe("stuck");
		expect(activeLens({ statuses: ["PENDING", "QUEUED"], lens: "" })).toBeNull();
		expect(activeLens({ statuses: [], lens: "" })).toBeNull();
	});
});

describe("event types", () => {
	it("trims and de-duplicates", () => {
		expect(cleanEventTypes([" a ", "a", "", "b"])).toEqual(["a", "b"]);
	});
	it("refuses more than the server's 10", () => {
		const eleven = Array.from({ length: MAX_EVENT_TYPES + 1 }, (_, i) => `t:${i}`);
		expect(validateFilters({ eventTypes: eleven })).toMatch(/At most 10/);
		expect(validateFilters({ eventTypes: eleven.slice(0, 10) })).toBeNull();
	});
	it("suggests the typed text first, then facets containing it, never chosen ones", () => {
		const facets = ["billing:invoice:created", "billing:invoice:paid", "shop:order:created"];
		expect(suggestEventTypes("invoice", facets, ["billing:invoice:paid"])).toEqual([
			"invoice",
			"billing:invoice:created",
		]);
		expect(suggestEventTypes("", facets, [])).toEqual(facets);
		expect(suggestEventTypes("shop:order:created", facets, [])).toEqual(["shop:order:created"]);
	});
});

describe("explainListError", () => {
	it("turns the API's codes into readable text", () => {
		expect(explainListError({ code: "TOO_MANY_EVENT_TYPES", message: "x" })).toMatch(/at most 10/);
		expect(explainListError({ code: "TOO_MANY_SUBSCRIPTION_IDS" })).toMatch(/50/);
		expect(explainListError({ code: "TOO_MANY_MATCHES" })).toMatch(/more specific/);
		expect(explainListError({ code: "WINDOW_TOO_LARGE" })).toMatch(/31 days/);
		expect(explainListError({ code: "INVALID_WINDOW" })).toMatch(/before it starts/);
	});
	it("falls back to the server's message", () => {
		expect(explainListError({ code: "WHATEVER", message: "boom" })).toBe("boom");
		expect(explainListError(null)).toMatch(/could not be loaded/);
	});
});

describe("requeue summary carries the new filters", () => {
	it("lists event type, subscription, connection and a non-default window", () => {
		const s = summarizeFilters({
			clients: [], applications: [], subdomains: [], aggregates: [], codes: [], statuses: [], search: "",
			messageGroup: "", from: "", to: "",
			eventTypes: ["a:b:c:d"], subscription: "orders*", connection: "billing", windowLabel: "Last 7 days",
		});
		expect(s).toEqual([
			{ label: "Event type", value: "a:b:c:d" },
			{ label: "Subscription", value: "orders*" },
			{ label: "Connection", value: "billing" },
			{ label: "Window", value: "Last 7 days" },
		]);
	});
});
