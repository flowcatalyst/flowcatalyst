/**
 * Filter plumbing for the dispatch job list: list filters -> API params
 * (testable without the component), quick lenses, input limits and readable
 * messages for the API's 400s. tests/dispatch-jobs-filters.test.ts pins it.
 */

import type { DispatchJobsListParams } from "@/api/dispatch-jobs";
import { normalizePreset, windowBounds } from "./jobWindow";

/** The server's cap on eventTypes (more is 400 TOO_MANY_EVENT_TYPES). */
export const MAX_EVENT_TYPES = 10;
/** A job older than this and still PENDING/QUEUED is "stuck". */
export const STUCK_AFTER_MS = 5 * 60_000;

export interface JobListFilters {
	clients: string[];
	applications: string[];
	subdomains: string[];
	aggregates: string[];
	codes: string[];
	statuses: string[];
	search: string;
	messageGroup: string;
	eventTypes: string[];
	/** Subscription name, exact or `prefix*`; sent as subscriptionName. */
	subscription: string;
	/** Connection code, exact or `prefix*`. */
	connection: string;
	/** Window preset key, "" = default. */
	window: string;
	/** Custom range, YYYY-MM-DD (whole days). */
	from: string;
	to: string;
	/** "stuck" adds the older-than-5-minutes cap to the PENDING/QUEUED statuses. */
	lens: string;
}

export const EMPTY_FILTERS: JobListFilters = {
	clients: [],
	applications: [],
	subdomains: [],
	aggregates: [],
	codes: [],
	statuses: [],
	search: "",
	messageGroup: "",
	eventTypes: [],
	subscription: "",
	connection: "",
	window: "",
	from: "",
	to: "",
	lens: "",
};

export type Lens = "failed" | "stuck";

export const FAILED_STATUSES: readonly string[] = ["FAILED", "ERROR"];
export const STUCK_STATUSES: readonly string[] = ["PENDING", "QUEUED"];

function sameSet(a: readonly string[], b: readonly string[]): boolean {
	return a.length === b.length && a.every((x) => b.includes(x));
}

/** The lens the filters currently amount to, or null. */
export function activeLens(f: Pick<JobListFilters, "statuses" | "lens">): Lens | null {
	if (sameSet(f.statuses, FAILED_STATUSES)) return "failed";
	if (f.lens === "stuck" && sameSet(f.statuses, STUCK_STATUSES)) return "stuck";
	return null;
}

/** What choosing a lens sets (the page applies it over the other filters it keeps). */
export function lensFilters(lens: Lens): Pick<JobListFilters, "statuses" | "lens"> {
	return lens === "failed"
		? { statuses: [...FAILED_STATUSES], lens: "" }
		: { statuses: [...STUCK_STATUSES], lens: "stuck" };
}

/** Trim, drop empties and duplicates, keep order. */
export function cleanEventTypes(list: string[]): string[] {
	const seen = new Set<string>();
	const out: string[] = [];
	for (const raw of list) {
		const v = raw.trim();
		if (v && !seen.has(v)) {
			seen.add(v);
			out.push(v);
		}
	}
	return out;
}

/** A message when the filters cannot be sent, else null. */
export function validateFilters(f: Pick<JobListFilters, "eventTypes">): string | null {
	if (cleanEventTypes(f.eventTypes).length > MAX_EVENT_TYPES) {
		return `At most ${MAX_EVENT_TYPES} event types can be filtered at once.`;
	}
	return null;
}

function nonEmpty<T>(a: T[]): T[] | undefined {
	return a.length ? a : undefined;
}

/** The list query for these filters. `size`/`sort` are the page's. */
export function filtersToParams(
	f: JobListFilters,
	opts: { size: number; sort: "createdAt.asc" | "createdAt.desc"; now: Date },
): DispatchJobsListParams {
	const stuck = activeLens(f) === "stuck";
	const { since, until } = windowBounds(
		{ preset: normalizePreset(f.window), from: f.from, to: f.to },
		opts.now,
		stuck ? new Date(opts.now.getTime() - STUCK_AFTER_MS) : undefined,
	);
	const subscription = f.subscription.trim();
	const connection = f.connection.trim();
	return {
		size: opts.size,
		clientIds: nonEmpty(f.clients),
		statuses: nonEmpty(f.statuses),
		applications: nonEmpty(f.applications),
		subdomains: nonEmpty(f.subdomains),
		aggregates: nonEmpty(f.aggregates),
		codes: nonEmpty(f.codes),
		source: f.search || undefined,
		messageGroup: f.messageGroup || undefined,
		eventTypes: nonEmpty(cleanEventTypes(f.eventTypes)),
		subscriptionName: subscription || undefined,
		connectionCode: connection || undefined,
		since,
		until,
		sort: opts.sort,
	};
}

/** The API error codes the filters can provoke, in words an operator can act on. */
export function explainListError(err: unknown): string {
	const e = err as { code?: string; message?: string } | null;
	const message = e?.message || "The dispatch jobs could not be loaded.";
	switch (e?.code) {
		case "TOO_MANY_EVENT_TYPES":
			return `Too many event types: at most ${MAX_EVENT_TYPES} at once. Remove some, or use a prefix search on the Code filter.`;
		case "TOO_MANY_SUBSCRIPTION_IDS":
			return "Too many subscriptions selected: at most 50 at once.";
		case "TOO_MANY_MATCHES":
			return "The subscription or connection text matches too many records. Make it more specific, for example a longer prefix before the *.";
		case "WINDOW_TOO_LARGE":
			return "That time window is too wide: the longest allowed is 31 days. Pick a shorter range.";
		case "INVALID_WINDOW":
			return "The window ends before it starts. Check the From and To dates.";
		case "INVALID_TIMESTAMP":
			return "A date in the window was not understood. Pick the dates again.";
		default:
			return message;
	}
}

/**
 * Autocomplete options for the event type filter: what was typed first (any
 * text is accepted, the facets only suggest), then the window's event types
 * containing it, never one already chosen.
 */
export function suggestEventTypes(query: string, facets: string[], selected: string[], limit = 20): string[] {
	const q = query.trim();
	const chosen = new Set(selected);
	const lower = q.toLowerCase();
	const matches = facets.filter((f) => !chosen.has(f) && f.toLowerCase().includes(lower));
	const out: string[] = [];
	if (q && !chosen.has(q)) out.push(q);
	for (const m of matches) {
		if (m !== q) out.push(m);
	}
	return out.slice(0, limit);
}
