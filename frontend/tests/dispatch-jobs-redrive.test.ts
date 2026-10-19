import { afterEach, describe, expect, it, vi } from "vitest";
import { dispatchJobsApi } from "@/api/dispatch-jobs";
import {
	initialRedrive,
	parseTypedCount,
	redriveOffered,
	redriveReady,
	resetRedrive,
	startRedrive,
} from "@/pages/dispatch-jobs/redriveConfirm";
import { requeueIdsOutcome, requeueOutcome } from "@/pages/dispatch-jobs/requeueByFilter";

describe("redrive double confirmation", () => {
	it("is offered only to holders, and only when COMPLETED jobs match", () => {
		expect(redriveOffered(true, 3)).toBe(true);
		expect(redriveOffered(false, 3)).toBe(false);
		expect(redriveOffered(true, 0)).toBe(false);
	});
	it("starts in review and is never ready there", () => {
		const s = initialRedrive();
		expect(s.phase).toBe("review");
		expect(redriveReady({ ...s, typedWord: "REDRIVE", typedCount: "5" }, 5)).toBe(false);
	});
	it("needs the exact word and the retyped count", () => {
		const s = startRedrive();
		expect(s.phase).toBe("confirm");
		expect(redriveReady(s, 5)).toBe(false);
		expect(redriveReady({ ...s, typedWord: "redrive", typedCount: "5" }, 5)).toBe(false);
		expect(redriveReady({ ...s, typedWord: "REDRIVE", typedCount: "4" }, 5)).toBe(false);
		expect(redriveReady({ ...s, typedWord: "REDRIVE", typedCount: "" }, 5)).toBe(false);
		expect(redriveReady({ ...s, typedWord: "REDRIVE", typedCount: "5" }, 5)).toBe(true);
		expect(redriveReady({ ...s, typedWord: "REDRIVE", typedCount: "1,250" }, 1250)).toBe(true);
	});
	it("never ready for a zero count", () => {
		expect(redriveReady({ ...startRedrive(), typedWord: "REDRIVE", typedCount: "0" }, 0)).toBe(false);
	});
	it("backing out (or a changed match) clears what was typed", () => {
		expect(resetRedrive()).toEqual({ phase: "review", typedWord: "", typedCount: "" });
	});
	it("parses typed counts strictly", () => {
		expect(parseTypedCount(" 12 ")).toBe(12);
		expect(parseTypedCount("12a")).toBeNaN();
		expect(parseTypedCount("")).toBeNaN();
	});
});

describe("request bodies", () => {
	afterEach(() => vi.unstubAllGlobals());
	function stubFetch() {
		const calls: { url: string; body: unknown }[] = [];
		vi.stubGlobal(
			"fetch",
			vi.fn(async (url: string, init: RequestInit) => {
				calls.push({ url, body: JSON.parse(String(init.body)) });
				return new Response("{}", { status: 200 });
			}),
		);
		return calls;
	}
	it("a plain by-filter requeue sends no redrive fields", async () => {
		const calls = stubFetch();
		await dispatchJobsApi.requeueByFilter({ statuses: "FAILED" }, 7);
		expect(calls[0]!.body).toEqual({ statuses: "FAILED", confirmCount: 7 });
	});
	it("a redrive sends the flag, the word and the redrive count", async () => {
		const calls = stubFetch();
		await dispatchJobsApi.requeueByFilter({ statuses: "COMPLETED" }, 12, true);
		expect(calls[0]!.body).toEqual({
			statuses: "COMPLETED",
			confirmCount: 12,
			redriveCompleted: true,
			confirmRedriveCompleted: "REDRIVE",
		});
	});
	it("by-ids requeue only adds the redrive fields when asked", async () => {
		const calls = stubFetch();
		await dispatchJobsApi.requeue(["a"]);
		await dispatchJobsApi.requeue(["a", "b"], true);
		expect(calls[0]!.body).toEqual({ ids: ["a"] });
		expect(calls[1]!.body).toEqual({ ids: ["a", "b"], redriveCompleted: true, confirmRedriveCompleted: "REDRIVE" });
	});
});

describe("outcomes", () => {
	it("by-ids: reset, redriven and skipped counts", () => {
		const o = requeueIdsOutcome({ requeued: 3, redriven: 1, skippedInFlight: 2, skippedCompleted: 0 });
		expect(o.severity).toBe("success");
		expect(o.summary).toBe("Requeued 3 jobs (1 COMPLETED job redriven)");
		expect(o.detail).toContain("2 jobs in flight");
	});
	it("by-ids: nothing reset is info, with why", () => {
		const o = requeueIdsOutcome({ requeued: 0, skippedCompleted: 4 });
		expect(o.severity).toBe("info");
		expect(o.detail).toContain("4 COMPLETED jobs left alone");
	});
	it("by-filter mentions redriven jobs", () => {
		expect(requeueOutcome({ requeued: 5, truncated: false, remaining: 0, redriven: 2 }).summary).toBe(
			"Requeued 5 jobs (2 COMPLETED jobs redriven)",
		);
	});
});
