/**
 * Event Types Resource
 *
 * Manage event type definitions and schemas.
 */

import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors";
import type { FlowCatalystClient } from "../client";
import * as sdk from "../generated/sdk.gen";
import type {
	GetApiAdminEventTypesResponse,
	GetApiAdminEventTypesByIdResponse,
	PostApiAdminEventTypesData,
	PutApiAdminEventTypesByIdData,
	PostApiAdminEventTypesByIdSchemasData,
	PostApiAdminEventTypesSyncData,
	PostApiAdminEventTypesSyncResponse,
	PaginationParams,
} from "../generated/types.gen";

export type EventTypeListResponse = GetApiAdminEventTypesResponse;
export type EventTypeResponse = GetApiAdminEventTypesByIdResponse;
export type CreateEventTypeRequest = PostApiAdminEventTypesData["body"];
export type UpdateEventTypeRequest = PutApiAdminEventTypesByIdData["body"];
export type SyncEventTypesResponse = PostApiAdminEventTypesSyncResponse;

export interface EventTypeFilters {
	status?: string;
	application?: string;
	clientId?: string;
}

/**
 * Event Types resource for managing event type definitions.
 */
export class EventTypesResource {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	/**
	 * List all event types with optional filters.
	 */
	list(
		filters?: EventTypeFilters,
		pagination?: PaginationParams,
	): ResultAsync<EventTypeListResponse, SdkError> {
		return this.client.request<EventTypeListResponse>((httpClient, headers) =>
			sdk.getApiAdminEventTypes({
				client: httpClient,
				headers,
				query: {
					pagination: pagination ?? {},
					...filters,
				},
			}),
		);
	}

	/**
	 * Get an event type by ID.
	 */
	get(id: string): ResultAsync<EventTypeResponse, SdkError> {
		return this.client.request<EventTypeResponse>((httpClient, headers) =>
			sdk.getApiAdminEventTypesById({
				client: httpClient,
				headers,
				path: { id },
			}),
		);
	}

	/**
	 * Create a new event type.
	 */
	create(
		data: CreateEventTypeRequest,
	): ResultAsync<EventTypeResponse, SdkError> {
		return this.client.request<EventTypeResponse>((httpClient, headers) =>
			sdk.postApiAdminEventTypes({
				client: httpClient,
				headers,
				body: data,
			}),
		);
	}

	/**
	 * Update an event type.
	 */
	update(
		id: string,
		data: UpdateEventTypeRequest,
	): ResultAsync<EventTypeResponse, SdkError> {
		return this.client.request<EventTypeResponse>((httpClient, headers) =>
			sdk.putApiAdminEventTypesById({
				client: httpClient,
				headers,
				path: { id },
				body: data,
			}),
		);
	}

	/**
	 * Add a schema version to an event type.
	 */
	addSchema(
		id: string,
		schema: PostApiAdminEventTypesByIdSchemasData["body"],
	): ResultAsync<EventTypeResponse, SdkError> {
		return this.client.request<EventTypeResponse>((httpClient, headers) =>
			sdk.postApiAdminEventTypesByIdSchemas({
				client: httpClient,
				headers,
				path: { id },
				body: schema,
			}),
		);
	}

	/**
	 * Delete an event type.
	 */
	delete(id: string): ResultAsync<unknown, SdkError> {
		return this.client.request<unknown>((httpClient, headers) =>
			sdk.deleteApiAdminEventTypesById({
				client: httpClient,
				headers,
				path: { id },
			}),
		);
	}

	/**
	 * Sync event types for an application.
	 */
	sync(
		applicationCode: string,
		eventTypes: PostApiAdminEventTypesSyncData["body"]["eventTypes"],
		removeUnlisted = false,
	): ResultAsync<SyncEventTypesResponse, SdkError> {
		return this.client.request<SyncEventTypesResponse>((httpClient, headers) =>
			sdk.postApiAdminEventTypesSync({
				client: httpClient,
				headers,
				body: { applicationCode, eventTypes },
				query: { removeUnlisted },
			}),
		);
	}
}
