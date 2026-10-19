/**
 * Dispatch jobs API client. No pagination — `msg_dispatch_jobs_read`
 * ingests at high rates, page navigation is meaningless. The endpoint
 * returns the most recent N rows matching the filters; configure with
 * `?size=` (default 50, max 1000).
 */

import { apiFetch } from "./client";
import type {
	DeliveryPlan as GenDeliveryPlan,
	DispatchJobFilterOptionsResponse,
	DispatchJobRead as GenDispatchJobRead,
	DispatchJobResponse as GenDispatchJobResponse,
	EditPayloadRequest,
	RequestSummary as GenRequestSummary,
	RequeueByFilterPreviewRequest,
	RequeueByFilterPreviewResponse,
	RequeueByFilterResponse,
	RequeueResponse,
	WindowResponse,
} from "./generated";

export type {
	EditPayloadRequest,
	RequeueByFilterPreviewResponse,
	RequeueByFilterResponse,
	RequeueResponse,
	WindowResponse,
};

/**
 * Confirmation that turns a requeue into a redrive of COMPLETED jobs. The
 * server also needs the dispatch-job:redrive-completed permission (403
 * otherwise) and a confirmCount of redriveEligibleCount.
 */
export const REDRIVE_CONFIRM_WORD = "REDRIVE";

// Response types alias the generated contract (api/openapi.lock.json) so
// `vue-tsc` fails on backend drift. Aliased under the historical names so
// pages keep their imports. The old hand-rolled row carried phantom fields
// (no `maxRetries` on the read row) and an index signature; the old
// filter-options shape ({value,label} arrays under applications/subdomains/
// aggregates) never matched the wire — the facets are plain string arrays
// under statuses/codes/clientIds/dispatchPoolIds/subscriptionIds/kinds.
export type DispatchJobRead = GenDispatchJobRead;
export type DispatchJobFilterOptions = DispatchJobFilterOptionsResponse;
/** The full job (write-side row): payload, metadata, attempts. */
export type DispatchJobDetail = GenDispatchJobResponse;
export type DispatchJobAttempt = NonNullable<GenDispatchJobResponse["attempts"]>[number];
/** What the platform sent on an attempt — see the Go RequestSummary. */
export type DeliveryRequestSummary = GenRequestSummary;
/** The "sign" action's answer: the delivery as it would go out right now. */
export type DeliveryPlan = GenDeliveryPlan;

export interface DispatchJobsListParams {
	size?: number;
	clientIds?: string[] | undefined;
	statuses?: string[] | undefined;
	applications?: string[] | undefined;
	subdomains?: string[] | undefined;
	aggregates?: string[] | undefined;
	codes?: string[] | undefined;
	source?: string | undefined;
	/** Exact message group. */
	messageGroup?: string | undefined;
	/** Event types (a job's code is its event type); at most 10. Served by an index that also orders by createdAt. */
	eventTypes?: string[] | undefined;
	/** Subscription ids; at most 50. */
	subscriptionIds?: string[] | undefined;
	/** Subscription code, exact; a trailing * makes it a prefix. Resolved to ids server-side. */
	subscriptionCode?: string | undefined;
	/** Subscription name, exact and case-insensitive; a trailing * makes it a prefix. */
	subscriptionName?: string | undefined;
	/** Jobs delivered through this connection (its subscriptions' jobs and function dispatches). */
	connectionId?: string | undefined;
	/** Connection code, exact; a trailing * makes it a prefix. */
	connectionCode?: string | undefined;
	/** RFC3339 lower bound on createdAt. */
	since?: string | undefined;
	/** RFC3339 upper bound on createdAt. */
	until?: string | undefined;
	/** createdAt sort direction; server default is newest-first. */
	sort?: "createdAt.asc" | "createdAt.desc" | undefined;
}

/**
 * The filter part of the list params, in the wire form the server reads both
 * as the list's query string and as the requeue-by-filter request body (same
 * names, plural fields comma-separated). One builder, so "requeue all
 * matching" sends exactly the filter the list was loaded with.
 */
export type DispatchJobFilterBody = RequeueByFilterPreviewRequest;

export function filterBody(params: DispatchJobsListParams): DispatchJobFilterBody {
	const body: DispatchJobFilterBody = {};
	if (params.clientIds?.length) body.clientIds = params.clientIds.join(",");
	if (params.statuses?.length) body.statuses = params.statuses.join(",");
	if (params.applications?.length) body.applications = params.applications.join(",");
	if (params.subdomains?.length) body.subdomains = params.subdomains.join(",");
	if (params.aggregates?.length) body.aggregates = params.aggregates.join(",");
	if (params.codes?.length) body.codes = params.codes.join(",");
	if (params.source) body.source = params.source;
	if (params.messageGroup) body.messageGroup = params.messageGroup;
	if (params.eventTypes?.length) body.eventTypes = params.eventTypes.join(",");
	if (params.subscriptionIds?.length) body.subscriptionIds = params.subscriptionIds.join(",");
	if (params.subscriptionCode) body.subscriptionCode = params.subscriptionCode;
	if (params.subscriptionName) body.subscriptionName = params.subscriptionName;
	if (params.connectionId) body.connectionId = params.connectionId;
	if (params.connectionCode) body.connectionCode = params.connectionCode;
	if (params.since) body.since = params.since;
	if (params.until) body.until = params.until;
	return body;
}

export function buildQuery(params: DispatchJobsListParams): string {
	const qp = new URLSearchParams();
	if (params.size != null) qp.set("size", String(params.size));
	for (const [k, v] of Object.entries(filterBody(params))) {
		if (typeof v === "string") qp.set(k, v);
	}
	if (params.sort) qp.set("sort", params.sort);
	const s = qp.toString();
	return s ? `?${s}` : "";
}

/**
 * The window the server applied to a list, from its X-Window-From / -To /
 * -Defaulted headers. null when the headers are absent (an older server).
 */
export function windowFromHeaders(h: Pick<Headers, "get">): WindowResponse | null {
	const from = h.get("X-Window-From");
	const to = h.get("X-Window-To");
	if (!from || !to) return null;
	return { from, to, defaulted: h.get("X-Window-Defaulted") === "true" };
}

export const dispatchJobsApi = {
	list(params: DispatchJobsListParams): Promise<DispatchJobRead[]> {
		return apiFetch(`/dispatch-jobs${buildQuery(params)}`);
	},
	/**
	 * The list plus the window the server applied. The caller shows errors
	 * (400 WINDOW_TOO_LARGE, TOO_MANY_* ...) inline, so the global toast is off.
	 */
	async listWithWindow(
		params: DispatchJobsListParams,
	): Promise<{ rows: DispatchJobRead[]; window: WindowResponse | null }> {
		let window: WindowResponse | null = null;
		const rows = await apiFetch<DispatchJobRead[]>(`/dispatch-jobs${buildQuery(params)}`, {
			suppressGlobalErrorToast: true,
			onResponse: (r) => {
				window = windowFromHeaders(r.headers);
			},
		});
		return { rows, window };
	},
	get(id: string): Promise<DispatchJobDetail> {
		return apiFetch(`/dispatch-jobs/${encodeURIComponent(id)}`);
	},
	attempts(id: string): Promise<DispatchJobAttempt[]> {
		return apiFetch(`/dispatch-jobs/${encodeURIComponent(id)}/attempts`);
	},
	filterOptions(): Promise<DispatchJobFilterOptions> {
		return apiFetch(`/dispatch-jobs/filter-options`);
	},
	// Reset jobs to PENDING so the scheduler re-dispatches them. Returns the
	// number actually reset (tenant-scoped server-side). Used by the list
	// page's per-row retry + bulk "requeue selected".
	// With `redrive` the COMPLETED ones are reset too (needs the
	// redrive-completed permission; 403 without it).
	requeue(ids: string[], redrive = false): Promise<RequeueResponse> {
		return apiFetch(`/dispatch-jobs/requeue`, {
			method: "POST",
			body: JSON.stringify(
				redrive ? { ids, redriveCompleted: true, confirmRedriveCompleted: REDRIVE_CONFIRM_WORD } : { ids },
			),
		});
	},
	// How many jobs the filter matches (by status) and how many a requeue
	// would reset for this caller (`eligibleCount`, tenant-scoped).
	previewRequeueByFilter(filter: DispatchJobFilterBody): Promise<RequeueByFilterPreviewResponse> {
		return apiFetch(`/dispatch-jobs/requeue-by-filter/preview`, {
			method: "POST",
			body: JSON.stringify(filter),
		});
	},
	// Reset every job the filter matches to PENDING, at most `confirmCount`
	// (the eligibleCount the user confirmed) and the server's 10,000 cap. A
	// 409 FILTER_MATCH_CHANGED means more jobs match now: nothing was reset,
	// re-preview and ask again. Errors are not toasted globally — the caller
	// handles the 409 in its dialog.
	// With `redrive`, confirmCount must be the preview's redriveEligibleCount
	// and COMPLETED jobs are reset as well.
	requeueByFilter(
		filter: DispatchJobFilterBody,
		confirmCount: number,
		redrive = false,
	): Promise<RequeueByFilterResponse> {
		return apiFetch(`/dispatch-jobs/requeue-by-filter`, {
			method: "POST",
			body: JSON.stringify({
				...filter,
				confirmCount,
				...(redrive ? { redriveCompleted: true, confirmRedriveCompleted: REDRIVE_CONFIRM_WORD } : {}),
			}),
			suppressGlobalErrorToast: true,
		});
	},
	// Replace the payload (and optionally the content type) of a PENDING or
	// FAILED job; status and attempts are unchanged — requeue it afterwards.
	// 409 NOT_EDITABLE means the job moved on (queued, delivered...); the
	// caller handles it, so the global error toast is suppressed.
	editPayload(id: string, body: EditPayloadRequest): Promise<DispatchJobDetail> {
		return apiFetch(`/dispatch-jobs/${encodeURIComponent(id)}/payload`, {
			method: "PUT",
			body: JSON.stringify(body),
			suppressGlobalErrorToast: true,
		});
	},
	// Cancel a FAILED job (an operator override: it will not be retried, and
	// a BLOCK_ON_ERROR group behind it is released). 409 when it is not FAILED.
	cancel(id: string): Promise<DispatchJobDetail> {
		return apiFetch(`/dispatch-jobs/${encodeURIComponent(id)}/cancel`, {
			method: "POST",
			suppressGlobalErrorToast: true,
		});
	},
	// Mark a FAILED job COMPLETED (it was delivered out of band). 409 when
	// it is not FAILED.
	complete(id: string): Promise<DispatchJobDetail> {
		return apiFetch(`/dispatch-jobs/${encodeURIComponent(id)}/complete`, {
			method: "POST",
			suppressGlobalErrorToast: true,
		});
	},
	// Dry run: which service account would sign, every header, and a real
	// signature over the real body — without delivering. Hand the timestamp,
	// signature and body to the subscriber's verify command to see whether
	// THEIR secret accepts it.
	sign(id: string): Promise<DeliveryPlan> {
		return apiFetch(`/dispatch-jobs/${encodeURIComponent(id)}/sign`, { method: "POST" });
	},
};
