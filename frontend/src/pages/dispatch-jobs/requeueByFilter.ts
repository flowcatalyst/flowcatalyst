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
	eventTypes?: string[];
	subscription?: string;
	connection?: string;
	/** Window preset label ("Last 7 days"), when a preset other than the default is chosen. */
	windowLabel?: string;
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
	list("Event type", f.eventTypes ?? []);
	if (f.subscription) out.push({ label: "Subscription", value: f.subscription });
	if (f.connection) out.push({ label: "Connection", value: f.connection });
	if (f.windowLabel) out.push({ label: "Window", value: f.windowLabel });
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

/**
 * What a requeue left alone, in words ("" when nothing was skipped): jobs in
 * flight (QUEUED, PROCESSING) are never reset; COMPLETED ones are left alone
 * unless the run was a confirmed redrive.
 */
export function requeueSkipNote(res: {
	skippedInFlight?: number;
	skippedCompleted?: number;
}): string {
	const parts: string[] = [];
	if (res.skippedInFlight) parts.push(`${plural(res.skippedInFlight, "job")} in flight (QUEUED or PROCESSING) left alone`);
	if (res.skippedCompleted) parts.push(`${plural(res.skippedCompleted, "COMPLETED job")} left alone`);
	return parts.length ? ` — ${parts.join("; ")}` : "";
}

/** The toast for a finished run. */
export function requeueOutcome(res: Pick<RequeueByFilterResponse, "requeued" | "truncated" | "remaining"> & {
	redriven?: number;
	skippedInFlight?: number;
	skippedCompleted?: number;
}): {
	severity: "success" | "warn" | "info";
	summary: string;
	detail: string;
} {
	const skipped = requeueSkipNote(res);
	const redriven = res.redriven ? ` (${plural(res.redriven, "COMPLETED job")} redriven)` : "";
	if (res.truncated) {
		return {
			severity: "warn",
			summary: `Requeued ${plural(res.requeued, "job")}${redriven}`,
			detail: `${res.remaining.toLocaleString()} remain — run it again to continue.${skipped}`,
		};
	}
	if (res.requeued === 0) {
		return { severity: "info", summary: "Nothing requeued", detail: `No matching job could be requeued.${skipped}` };
	}
	return {
		severity: "success",
		summary: `Requeued ${plural(res.requeued, "job")}${redriven}`,
		detail: `${plural(res.requeued, "dispatch job")} reset to PENDING.${skipped}`,
	};
}

/**
 * The toast for a requeue by ids: what was reset, what the guard left alone.
 * (The response lists every skipped id with its reason; the counts are enough
 * for a toast.)
 */
export function requeueIdsOutcome(res: {
	requeued: number;
	redriven?: number;
	skippedInFlight?: number;
	skippedCompleted?: number;
}): { severity: "success" | "info"; summary: string; detail: string } {
	const skipped = requeueSkipNote(res);
	if (res.requeued === 0) {
		return { severity: "info", summary: "Nothing requeued", detail: `No selected job could be requeued.${skipped}` };
	}
	const redriven = res.redriven ? ` (${plural(res.redriven, "COMPLETED job")} redriven)` : "";
	return {
		severity: "success",
		summary: `Requeued ${plural(res.requeued, "job")}${redriven}`,
		detail: `${plural(res.requeued, "dispatch job")} reset to PENDING${skipped}`,
	};
}
