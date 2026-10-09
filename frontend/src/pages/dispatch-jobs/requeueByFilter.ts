/**
 * Plain logic behind the dispatch-job list's "requeue all matching" flow,
 * kept out of the components so tests/dispatch-jobs-requeue.test.ts can pin it.
 */

import type { RequeueByFilterResponse } from "@/api/dispatch-jobs";

/**
 * The statuses a job is "in error" in: FAILED, plus ERROR — its legacy alias,
 * still a real stored value that the platform treats as failed everywhere.
 */
export const ERROR_STATUSES: readonly string[] = ["FAILED", "ERROR"];

/** The server's cap on one requeue-by-filter run (default and maximum). */
export const REQUEUE_BY_FILTER_MAX = 10_000;

export type TagSeverity = "success" | "info" | "warn" | "danger" | "secondary" | "contrast" | undefined;

export function statusSeverity(status: string): TagSeverity {
	switch (status) {
		case "COMPLETED":
			return "success";
		case "PENDING":
		case "QUEUED":
			return "info";
		case "PROCESSING":
			return "warn";
		case "FAILED":
		case "ERROR":
			return "danger";
		default:
			return "secondary";
	}
}

/** The list page's filter refs, as plain values. */
export interface ListFilterValues {
	clients: string[];
	applications: string[];
	subdomains: string[];
	aggregates: string[];
	codes: string[];
	statuses: string[];
	search: string;
	messageGroup: string;
	/** YYYY-MM-DD, whole-day lower bound. */
	from: string;
	/** YYYY-MM-DD, whole-day upper bound (inclusive). */
	to: string;
}

export interface FilterSummaryItem {
	label: string;
	value: string;
}

/**
 * The active filters as label/value lines — what the confirm dialog shows so
 * the user sees exactly which jobs "all matching" means. Empty = no filter.
 */
export function summarizeFilters(
	f: ListFilterValues,
	clientLabel: (id: string) => string = (id) => id,
): FilterSummaryItem[] {
	const out: FilterSummaryItem[] = [];
	const list = (label: string, values: string[], map: (v: string) => string = (v) => v) => {
		if (values.length) out.push({ label, value: values.map(map).join(", ") });
	};
	list("Client", f.clients, clientLabel);
	list("Application", f.applications);
	list("Subdomain", f.subdomains);
	list("Aggregate", f.aggregates);
	list("Code", f.codes);
	list("Status", f.statuses);
	if (f.search) out.push({ label: "Source", value: f.search });
	if (f.messageGroup) out.push({ label: "Message group", value: f.messageGroup });
	if (f.from) out.push({ label: "Created from", value: f.from });
	if (f.to) out.push({ label: "Created to", value: f.to });
	return out;
}

/**
 * Gmail-style selection banner over the grid:
 *  - "offer":    every row on a full page is selected and more may match —
 *                offer to select all matching (count may still be loading);
 *  - "selected": the user took the offer;
 *  - "hidden":   otherwise (a short page already holds every match).
 */
export function selectAllBannerState(o: {
	pageRows: number;
	selectedRows: number;
	pageSize: number;
	matchCount: number | null;
	allMatchingSelected: boolean;
}): "hidden" | "offer" | "selected" {
	if (o.allMatchingSelected) return "selected";
	if (o.pageRows === 0 || o.selectedRows !== o.pageRows || o.pageRows < o.pageSize) return "hidden";
	if (o.matchCount !== null && o.matchCount <= o.pageRows) return "hidden";
	return "offer";
}

export function plural(n: number, word: string): string {
	return `${n.toLocaleString()} ${word}${n === 1 ? "" : "s"}`;
}

/** The toast for a finished run. */
export function requeueOutcome(res: RequeueByFilterResponse): {
	severity: "success" | "warn" | "info";
	summary: string;
	detail: string;
} {
	if (res.truncated) {
		return {
			severity: "warn",
			summary: `Requeued ${plural(res.requeued, "job")}`,
			detail: `${res.remaining.toLocaleString()} remain — run it again to continue.`,
		};
	}
	if (res.requeued === 0) {
		return { severity: "info", summary: "Nothing requeued", detail: "No matching job could be requeued." };
	}
	return {
		severity: "success",
		summary: `Requeued ${plural(res.requeued, "job")}`,
		detail: `${plural(res.requeued, "dispatch job")} reset to PENDING.`,
	};
}
