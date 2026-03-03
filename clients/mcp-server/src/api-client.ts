/**
 * FlowCatalyst API Client
 *
 * Thin HTTP wrapper for the FlowCatalyst admin API.
 */

import type { Config } from "./config.js";
import type { TokenManager } from "./auth.js";

export interface EventTypeResponse {
	id: string;
	code: string;
	name: string;
	description: string | null;
	application: string | null;
	subdomain: string | null;
	aggregate: string | null;
	event: string | null;
	specVersions: SpecVersionResponse[];
	status: string;
	source: string;
	clientScoped: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface SpecVersionResponse {
	id: string;
	version: string;
	mimeType: string;
	schemaContent: unknown;
	schemaType: string;
	status: string;
	createdAt: string;
	updatedAt: string;
}

export interface EventTypesListResponse {
	eventTypes: EventTypeResponse[];
	total: number;
}

export interface SubscriptionResponse {
	id: string;
	name: string;
	description: string | null;
	eventTypeCode: string;
	status: string;
	endpoint: string | null;
	createdAt: string;
	updatedAt: string;
}

export interface SubscriptionsListResponse {
	subscriptions: SubscriptionResponse[];
	total: number;
}

export interface CodegenResponse {
	code: string;
	language: string;
	eventTypeId: string;
	eventCode: string;
	schemaVersion: string;
}

export class ApiClient {
	private readonly baseApiUrl: string;

	constructor(
		config: Config,
		private readonly tokenManager: TokenManager,
	) {
		this.baseApiUrl = `${config.baseUrl}/api/admin`;
	}

	async listEventTypes(filters?: {
		status?: string;
		application?: string;
		subdomain?: string;
		aggregate?: string;
	}): Promise<EventTypesListResponse> {
		const params = new URLSearchParams();
		if (filters?.status) params.set("status", filters.status);
		if (filters?.application) params.set("application", filters.application);
		if (filters?.subdomain) params.set("subdomain", filters.subdomain);
		if (filters?.aggregate) params.set("aggregate", filters.aggregate);

		const query = params.toString();
		const url = `${this.baseApiUrl}/event-types${query ? `?${query}` : ""}`;
		return this.get<EventTypesListResponse>(url);
	}

	async getEventType(id: string): Promise<EventTypeResponse> {
		return this.get<EventTypeResponse>(
			`${this.baseApiUrl}/event-types/${encodeURIComponent(id)}`,
		);
	}

	async generateCode(
		eventTypeId: string,
		language: string,
		version?: string,
	): Promise<CodegenResponse> {
		const body: Record<string, string> = { language };
		if (version) body["version"] = version;

		return this.post<CodegenResponse>(
			`${this.baseApiUrl}/event-types/${encodeURIComponent(eventTypeId)}/codegen`,
			body,
		);
	}

	async listSubscriptions(): Promise<SubscriptionsListResponse> {
		return this.get<SubscriptionsListResponse>(
			`${this.baseApiUrl}/subscriptions`,
		);
	}

	async getSubscription(id: string): Promise<SubscriptionResponse> {
		return this.get<SubscriptionResponse>(
			`${this.baseApiUrl}/subscriptions/${encodeURIComponent(id)}`,
		);
	}

	private async get<T>(url: string): Promise<T> {
		const token = await this.tokenManager.getAccessToken();
		const response = await fetch(url, {
			headers: {
				Authorization: `Bearer ${token}`,
				Accept: "application/json",
			},
		});

		if (!response.ok) {
			throw new Error(`API request failed: ${response.status} ${url}`);
		}

		return (await response.json()) as T;
	}

	private async post<T>(url: string, body: unknown): Promise<T> {
		const token = await this.tokenManager.getAccessToken();
		const response = await fetch(url, {
			method: "POST",
			headers: {
				Authorization: `Bearer ${token}`,
				"Content-Type": "application/json",
				Accept: "application/json",
			},
			body: JSON.stringify(body),
		});

		if (!response.ok) {
			throw new Error(`API request failed: ${response.status} ${url}`);
		}

		return (await response.json()) as T;
	}
}
