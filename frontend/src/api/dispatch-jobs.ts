/**
 * Dispatch jobs API client. Cursor-paginated; backend keysets on
 * `(created_at, id) DESC` and never counts.
 */

import { apiFetch } from "./client";

export interface DispatchJobRead {
	id: string;
	eventId: string;
	subscriptionId: string;
	clientId?: string | null;
	clientIdentifier?: string | null;
	application?: string | null;
	subdomain?: string | null;
	aggregate?: string | null;
	code?: string | null;
	source?: string | null;
	subject?: string | null;
	status: string;
	dispatchMode?: string | null;
	priority?: number | null;
	correlationId?: string | null;
	scheduledFor?: string | null;
	createdAt: string;
	updatedAt: string;
	completedAt?: string | null;
	lastAttemptAt?: string | null;
	attemptCount?: number | null;
	[key: string]: unknown;
}

export interface DispatchJobsCursorPage {
	items: DispatchJobRead[];
	hasMore: boolean;
	nextCursor?: string;
}

export interface DispatchJobsListParams {
	after?: string | undefined;
	size?: number;
	clientIds?: string[] | undefined;
	statuses?: string[] | undefined;
	applications?: string[] | undefined;
	subdomains?: string[] | undefined;
	aggregates?: string[] | undefined;
	codes?: string[] | undefined;
	source?: string | undefined;
}

export interface DispatchJobFilterOptions {
	applications: { value: string; label: string }[];
	subdomains: { value: string; label: string }[];
	aggregates: { value: string; label: string }[];
	codes: { value: string; label: string }[];
	statuses: { value: string; label: string }[];
}

function buildQuery(params: DispatchJobsListParams): string {
	const qp = new URLSearchParams();
	if (params.after) qp.set("after", params.after);
	if (params.size != null) qp.set("size", String(params.size));
	if (params.clientIds?.length) qp.set("clientIds", params.clientIds.join(","));
	if (params.statuses?.length) qp.set("statuses", params.statuses.join(","));
	if (params.applications?.length) qp.set("applications", params.applications.join(","));
	if (params.subdomains?.length) qp.set("subdomains", params.subdomains.join(","));
	if (params.aggregates?.length) qp.set("aggregates", params.aggregates.join(","));
	if (params.codes?.length) qp.set("codes", params.codes.join(","));
	if (params.source) qp.set("source", params.source);
	const s = qp.toString();
	return s ? `?${s}` : "";
}

export const dispatchJobsApi = {
	list(params: DispatchJobsListParams): Promise<DispatchJobsCursorPage> {
		return apiFetch(`/dispatch-jobs${buildQuery(params)}`);
	},
	filterOptions(): Promise<DispatchJobFilterOptions> {
		return apiFetch(`/dispatch-jobs/filter-options`);
	},
};
