import { apiFetch } from "./client";
import type {
	AliasResponse,
	CreatedResponse,
	FunctionResponse,
	LimitsDto,
	OffsetPageFunctionResponse as GenOffsetPageFunctionResponse,
	PutAliasResponse,
	SettingResponse,
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
}

export interface UpdateFunctionRequest {
	description?: string;
	pool?: string;
	/** Clears an explicit pool override back to the function's implied pool. */
	clearPool?: boolean;
	warm?: boolean;
	limits?: FunctionLimits;
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
	create(data: CreateFunctionRequest): Promise<CreatedResponse> {
		return apiFetch("/functions", {
			method: "POST",
			body: JSON.stringify(data),
		});
	},

	update(id: string, data: UpdateFunctionRequest): Promise<void> {
		return apiFetch(`/functions/${id}`, {
			method: "PATCH",
			body: JSON.stringify(data),
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
};
