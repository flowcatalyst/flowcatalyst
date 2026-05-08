/**
 * Events API client. Cursor-paginated — `msg_events_read` can be billions
 * of rows, so the backend keysets on `(time, id) DESC` and never counts.
 */

import { apiFetch } from "./client";

export interface EventRead {
	id: string;
	type: string;
	source: string;
	subject?: string | null;
	time: string;
	application?: string | null;
	subdomain?: string | null;
	aggregate?: string | null;
	messageGroup?: string | null;
	correlationId?: string | null;
	clientId?: string | null;
	projectedAt: string;
}

export interface EventDetail extends EventRead {
	specVersion?: string;
	deduplicationId?: string;
	causationId?: string;
	data?: unknown;
	contextData?: { key: string; value: string }[];
}

export interface EventsCursorPage {
	items: EventRead[];
	hasMore: boolean;
	nextCursor?: string;
}

export interface EventsListParams {
	after?: string | undefined;
	size?: number;
	clientIds?: string[] | undefined;
	applications?: string[] | undefined;
	subdomains?: string[] | undefined;
	aggregates?: string[] | undefined;
	types?: string[] | undefined;
	correlationId?: string | undefined;
	source?: string | undefined;
}

export interface EventFilterOptions {
	applications: { value: string; label: string }[];
	subdomains: { value: string; label: string }[];
	eventTypes: { value: string; label: string }[];
}

function buildQuery(params: EventsListParams): string {
	const qp = new URLSearchParams();
	if (params.after) qp.set("after", params.after);
	if (params.size != null) qp.set("size", String(params.size));
	if (params.clientIds?.length) qp.set("clientIds", params.clientIds.join(","));
	if (params.applications?.length) qp.set("applications", params.applications.join(","));
	if (params.subdomains?.length) qp.set("subdomains", params.subdomains.join(","));
	if (params.aggregates?.length) qp.set("aggregates", params.aggregates.join(","));
	if (params.types?.length) qp.set("types", params.types.join(","));
	if (params.correlationId) qp.set("correlationId", params.correlationId);
	if (params.source) qp.set("source", params.source);
	const s = qp.toString();
	return s ? `?${s}` : "";
}

export const eventsApi = {
	list(params: EventsListParams): Promise<EventsCursorPage> {
		return apiFetch(`/events${buildQuery(params)}`);
	},
	get(id: string): Promise<EventDetail> {
		return apiFetch(`/events/${encodeURIComponent(id)}`);
	},
	filterOptions(): Promise<EventFilterOptions> {
		return apiFetch(`/events/filter-options`);
	},
};
