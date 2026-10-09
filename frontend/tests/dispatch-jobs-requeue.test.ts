/**
 * Pure-logic tests for the dispatch-job list's "requeue all matching" flow:
 * the filter body sent to requeue-by-filter is the list's own filter
 * (src/api/dispatch-jobs.ts), and the page helpers in
 * src/pages/dispatch-jobs/requeueByFilter.ts.
 */

import { describe, expect, it } from "vitest";
import { buildQuery, filterBody, type DispatchJobsListParams } from "@/api/dispatch-jobs";
import {
	ERROR_STATUSES,
	requeueOutcome,
	selectAllBannerState,
	summarizeFilters,
	type ListFilterValues,
} from "@/pages/dispatch-jobs/requeueByFilter";

const params: DispatchJobsListParams = {
	size: 200,
	clientIds: ["clt_a", "clt_b"],
	statuses: ["FAILED", "ERROR"],
	applications: ["app"],
	subdomains: undefined,
	aggregates: [],
	codes: ["app:sub:agg:ev"],
	source: "src",
	messageGroup: "g1",
	since: "2026-10-01T00:00:00.000Z",
	until: "2026-10-02T00:00:00.000Z",
	sort: "createdAt.asc",
};

describe("filterBody", () => {
	it("is the list's filter in wire form: CSV plurals, no paging or sort", () => {
		expect(filterBody(params)).toEqual({
			clientIds: "clt_a,clt_b",
			statuses: "FAILED,ERROR",
			applications: "app",
			codes: "app:sub:agg:ev",
			source: "src",
			messageGroup: "g1",
			since: "2026-10-01T00:00:00.000Z",
			until: "2026-10-02T00:00:00.000Z",
		});
	});

	it("is empty for no filter", () => {
		expect(filterBody({ size: 50, sort: "createdAt.desc" })).toEqual({});
	});

	it("carries exactly the filter fields the list query carries", () => {
		const q = new URLSearchParams(buildQuery(params).slice(1));
		q.delete("size");
		q.delete("sort");
		expect(Object.fromEntries(q)).toEqual(filterBody(params));
	});
});

const noFilters: ListFilterValues = {
	clients: [],
	applications: [],
	subdomains: [],
	aggregates: [],
	codes: [],
	statuses: [],
	search: "",
	messageGroup: "",
	from: "",
	to: "",
};

describe("summarizeFilters", () => {
	it("is empty with no filter", () => {
		expect(summarizeFilters(noFilters)).toEqual([]);
	});

	it("lists each active filter, clients by name", () => {
		const labels: Record<string, string> = { clt_a: "Acme" };
		expect(
			summarizeFilters(
				{ ...noFilters, clients: ["clt_a", "clt_x"], statuses: [...ERROR_STATUSES], messageGroup: "g1", from: "2026-10-01" },
				(id) => labels[id] ?? id,
			),
		).toEqual([
			{ label: "Client", value: "Acme, clt_x" },
			{ label: "Status", value: "FAILED, ERROR" },
			{ label: "Message group", value: "g1" },
			{ label: "Created from", value: "2026-10-01" },
		]);
	});
});

describe("selectAllBannerState", () => {
	const base = { pageRows: 200, selectedRows: 200, pageSize: 200, matchCount: null, allMatchingSelected: false };

	it("offers when a full page is all selected and the count is unknown or larger", () => {
		expect(selectAllBannerState(base)).toBe("offer");
		expect(selectAllBannerState({ ...base, matchCount: 5000 })).toBe("offer");
	});

	it("hides when the page holds every match", () => {
		expect(selectAllBannerState({ ...base, matchCount: 200 })).toBe("hidden");
		expect(selectAllBannerState({ ...base, pageRows: 37, selectedRows: 37 })).toBe("hidden");
	});

	it("hides unless every row is selected", () => {
		expect(selectAllBannerState({ ...base, selectedRows: 199 })).toBe("hidden");
		expect(selectAllBannerState({ ...base, pageRows: 0, selectedRows: 0 })).toBe("hidden");
	});

	it("shows the selected state once taken", () => {
		expect(selectAllBannerState({ ...base, selectedRows: 3, allMatchingSelected: true })).toBe("selected");
	});
});

describe("requeueOutcome", () => {
	it("tells the user to run again when truncated", () => {
		const o = requeueOutcome({ requeued: 10000, truncated: true, remaining: 2500 });
		expect(o.severity).toBe("warn");
		expect(o.summary).toBe(`Requeued ${(10000).toLocaleString()} jobs`);
		expect(o.detail).toBe(`${(2500).toLocaleString()} remain — run it again to continue.`);
	});

	it("reports a complete run", () => {
		expect(requeueOutcome({ requeued: 1, truncated: false, remaining: 0 })).toEqual({
			severity: "success",
			summary: "Requeued 1 job",
			detail: "1 dispatch job reset to PENDING.",
		});
	});

	it("reports nothing requeued", () => {
		expect(requeueOutcome({ requeued: 0, truncated: false, remaining: 0 }).severity).toBe("info");
	});
});
