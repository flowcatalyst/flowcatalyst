import { apiFetch } from "./client";
import type {
	AliasResponse,
	CreatedResponse,
	FunctionResponse,
	LimitsDto,
	OffsetPageFunctionResponse as GenOffsetPageFunctionResponse,
	PutAliasResponse,
	SettingResponse,
	TraceContextResponse,
	TraceContextsResponse,
	TraceLogResponse,
	TraceRecordResponse,
	VersionResponse,
	WiringResponse,
} from "./generated";

// Response types alias the generated contract (api/openapi.lock.json) so
// `vue-tsc` fails on backend drift. Aliased under module-local names so
// pages get a stable vocabulary independent of the generator's naming.
export type FunctionRecord = FunctionResponse;
export type FunctionListResponse = GenOffsetPageFunctionResponse;
export type FunctionVersion = VersionResponse;
export type FunctionAlias = AliasResponse;
export type FunctionSetting = SettingResponse;
export type FunctionLimits = LimitsDto;
export type FunctionWiring = WiringResponse;
export type PutFunctionAliasResponse = PutAliasResponse;

export type TraceContext = TraceContextResponse;
export type TraceContextList = TraceContextsResponse;
export type TraceLogPage = TraceLogResponse;
export type TraceLogRecord = TraceRecordResponse;

/** Query of GET /functions/{id}/trace-log; empty values are not sent. */
export interface TraceLogQuery {
	context?: string;
	kind?: "trace" | "http" | "log";
	level?: string;
	reference?: string;
	correlationId?: string;
	invocationId?: string;
	version?: number;
	since?: string;
	until?: string;
	limit?: number;
	cursor?: string;
}

/** `fng_settings.kind` — see docs/function-runner-plan.md §8.1. */
export type FunctionSettingKind = "CONFIG" | "SECRET" | "DB";

export interface CreateFunctionRequest {
	applicationId: string;
	/** Lowercase, alphanumeric, hyphens; matches ^[a-z][a-z0-9-]{0,62}$ */
	name: string;
	clientId?: string;
	description?: string;
	pool?: string;
	warm?: boolean;
	limits?: FunctionLimits;
	/** Service account (id or code) that verifies webhook deliveries; unset = the application's account. */
	serviceAccountId?: string;
}

export interface UpdateFunctionRequest {
	description?: string;
	pool?: string;
	/** Clears an explicit pool override back to the function's implied pool. */
	clearPool?: boolean;
	warm?: boolean;
	limits?: FunctionLimits;
	/** Service account (id or code); mutually exclusive with `clearServiceAccount`. */
	serviceAccountId?: string;
	/** Reverts to the application's linked account. */
	clearServiceAccount?: boolean;
}

export interface ListFunctionsFilters {
	applicationId?: string;
	clientId?: string;
	/** Prefix match against `address`, e.g. an application code. */
	addressPrefix?: string;
	page?: number;
	pageSize?: number;
}

export const functionsApi = {
	list(filters: ListFunctionsFilters = {}): Promise<FunctionListResponse> {
		const params = new URLSearchParams();
		if (filters.applicationId) params.set("applicationId", filters.applicationId);
		if (filters.clientId) params.set("clientId", filters.clientId);
		if (filters.addressPrefix) params.set("addressPrefix", filters.addressPrefix);
		if (filters.page !== undefined) params.set("page", String(filters.page));
		if (filters.pageSize !== undefined) params.set("pageSize", String(filters.pageSize));

		const query = params.toString();
		return apiFetch(`/functions${query ? `?${query}` : ""}`);
	},

	get(id: string): Promise<FunctionRecord> {
		return apiFetch(`/functions/${id}`);
	},

	/** POST /functions returns the standard created envelope `{ id }`, not the full record. */
	create(
		data: CreateFunctionRequest,
		opts?: { suppressGlobalErrorToast?: boolean },
	): Promise<CreatedResponse> {
		return apiFetch("/functions", {
			method: "POST",
			body: JSON.stringify(data),
			...opts,
		});
	},

	update(
		id: string,
		data: UpdateFunctionRequest,
		opts?: { suppressGlobalErrorToast?: boolean },
	): Promise<void> {
		return apiFetch(`/functions/${id}`, {
			method: "PATCH",
			body: JSON.stringify(data),
			...opts,
		});
	},

	/** Cascades versions, aliases, wiring and artifacts on the backend. */
	delete(id: string): Promise<void> {
		return apiFetch(`/functions/${id}`, {
			method: "DELETE",
		});
	},

	listVersions(id: string): Promise<FunctionVersion[]> {
		return apiFetch(`/functions/${id}/versions`);
	},

	getVersion(id: string, number: number): Promise<FunctionVersion> {
		return apiFetch(`/functions/${id}/versions/${number}`);
	},

	/** Refused as RETIRE_IN_USE while an alias still points at the version. */
	retireVersion(id: string, number: number): Promise<FunctionVersion> {
		return apiFetch(`/functions/${id}/versions/${number}/retire`, {
			method: "POST",
		});
	},

	listAliases(id: string): Promise<FunctionAlias[]> {
		return apiFetch(`/functions/${id}/aliases`);
	},

	/**
	 * Points `name` at `version`. Promoting `live` materialises wiring
	 * (dispatch pool, subscriptions, scheduled jobs) — the response's
	 * `wiring` summarises what changed. Refuses a non-READY version.
	 */
	putAlias(id: string, name: string, version: number): Promise<PutFunctionAliasResponse> {
		return apiFetch(`/functions/${id}/aliases/${name}`, {
			method: "PUT",
			body: JSON.stringify({ version }),
		});
	},

	/** Removing `live` unwires subscriptions/schedules; the response carries that wiring delta. */
	deleteAlias(id: string, name: string): Promise<FunctionWiring> {
		return apiFetch(`/functions/${id}/aliases/${name}`, {
			method: "DELETE",
		});
	},

	listSettings(id: string): Promise<FunctionSetting[]> {
		return apiFetch(`/functions/${id}/settings`);
	},

	putConfig(id: string, key: string, value: string): Promise<FunctionSetting> {
		return apiFetch(`/functions/${id}/config/${encodeURIComponent(key)}`, {
			method: "PUT",
			body: JSON.stringify({ value }),
		});
	},

	deleteConfig(id: string, key: string): Promise<void> {
		return apiFetch(`/functions/${id}/config/${encodeURIComponent(key)}`, {
			method: "DELETE",
		});
	},

	/** Secrets are write-only — the response never carries the value back. */
	putSecret(id: string, key: string, value: string): Promise<FunctionSetting> {
		return apiFetch(`/functions/${id}/secrets/${encodeURIComponent(key)}`, {
			method: "PUT",
			body: JSON.stringify({ value }),
		});
	},

	deleteSecret(id: string, key: string): Promise<void> {
		return apiFetch(`/functions/${id}/secrets/${encodeURIComponent(key)}`, {
			method: "DELETE",
		});
	},

	/** A DB binding's value is a DSN — write-only, same as a secret. */
	putDb(id: string, name: string, value: string): Promise<FunctionSetting> {
		return apiFetch(`/functions/${id}/db/${encodeURIComponent(name)}`, {
			method: "PUT",
			body: JSON.stringify({ value }),
		});
	},

	deleteDb(id: string, name: string): Promise<void> {
		return apiFetch(`/functions/${id}/db/${encodeURIComponent(name)}`, {
			method: "DELETE",
		});
	},

	/** Declared trace contexts plus the built-in `http`, with their window state. */
	listTraceContexts(id: string): Promise<TraceContextList> {
		return apiFetch(`/functions/${id}/trace`);
	},

	/** Opens a window of `minutes` (capped per context); `http` also needs trace:view-bodies. */
	enableTrace(
		id: string,
		context: string,
		data: { minutes: number; sampleRate: number },
		opts?: { suppressGlobalErrorToast?: boolean },
	): Promise<TraceContext> {
		return apiFetch(`/functions/${id}/trace/${encodeURIComponent(context)}`, {
			method: "PUT",
			body: JSON.stringify(data),
			...opts,
		});
	},

	disableTrace(id: string, context: string): Promise<void> {
		return apiFetch(`/functions/${id}/trace/${encodeURIComponent(context)}`, {
			method: "DELETE",
		});
	},

	/** Newest first; pass the previous page's `nextCursor` as `cursor` for older records. */
	listTraceLog(
		id: string,
		query: TraceLogQuery = {},
		opts?: { suppressGlobalErrorToast?: boolean },
	): Promise<TraceLogPage> {
		const params = new URLSearchParams();
		for (const [key, value] of Object.entries(query)) {
			if (value !== undefined && value !== "") params.set(key, String(value));
		}
		const qs = params.toString();
		return apiFetch(`/functions/${id}/trace-log${qs ? `?${qs}` : ""}`, opts);
	},
};
