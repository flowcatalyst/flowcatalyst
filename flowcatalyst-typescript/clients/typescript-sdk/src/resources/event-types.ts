/**
 * Event Types Resource
 *
 * Manage event type definitions and schemas.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type {
  GetApiAdminEventTypesResponse,
  GetApiAdminEventTypesByIdResponse,
  PostApiAdminEventTypesData,
  PatchApiAdminEventTypesByIdData,
  PostApiAdminEventTypesByIdSchemasData,
  PostApiAdminEventTypesSyncData,
  PostApiAdminEventTypesSyncResponse,
  GetApiAdminEventTypesFiltersApplicationsResponse,
} from '../generated/types.gen';

export type EventTypeListResponse = GetApiAdminEventTypesResponse;
export type EventTypeResponse = GetApiAdminEventTypesByIdResponse;
export type CreateEventTypeRequest = PostApiAdminEventTypesData['body'];
export type UpdateEventTypeRequest = PatchApiAdminEventTypesByIdData['body'];
export type SyncEventTypesResponse = PostApiAdminEventTypesSyncResponse;
export type FilterOptionsResponse = GetApiAdminEventTypesFiltersApplicationsResponse;

export interface EventTypeFilters {
  status?: string;
  application?: string[];
  subdomain?: string[];
  aggregate?: string[];
}

/**
 * Event Types resource for managing event type definitions.
 */
export class EventTypesResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all event types with optional filters.
   */
  list(filters?: EventTypeFilters): ResultAsync<EventTypeListResponse, SdkError> {
    return this.client.request<EventTypeListResponse>((httpClient, headers) =>
      sdk.getApiAdminEventTypes({
        client: httpClient,
        headers,
        query: filters,
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
  create(data: CreateEventTypeRequest): ResultAsync<EventTypeResponse, SdkError> {
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
  update(id: string, data: UpdateEventTypeRequest): ResultAsync<EventTypeResponse, SdkError> {
    return this.client.request<EventTypeResponse>((httpClient, headers) =>
      sdk.patchApiAdminEventTypesById({
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
    schema: PostApiAdminEventTypesByIdSchemasData['body'],
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
   * Finalise a schema version (FINALISING -> CURRENT).
   */
  finaliseSchema(id: string, version: string): ResultAsync<EventTypeResponse, SdkError> {
    return this.client.request<EventTypeResponse>((httpClient, headers) =>
      sdk.postApiAdminEventTypesByIdSchemasByVersionFinalise({
        client: httpClient,
        headers,
        path: { id, version },
      }),
    );
  }

  /**
   * Deprecate a schema version (CURRENT -> DEPRECATED).
   */
  deprecateSchema(id: string, version: string): ResultAsync<EventTypeResponse, SdkError> {
    return this.client.request<EventTypeResponse>((httpClient, headers) =>
      sdk.postApiAdminEventTypesByIdSchemasByVersionDeprecate({
        client: httpClient,
        headers,
        path: { id, version },
      }),
    );
  }

  /**
   * Archive an event type.
   */
  archive(id: string): ResultAsync<EventTypeResponse, SdkError> {
    return this.client.request<EventTypeResponse>((httpClient, headers) =>
      sdk.postApiAdminEventTypesByIdArchive({
        client: httpClient,
        headers,
        path: { id },
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
   * Get distinct application names for filtering.
   */
  filterApplications(): ResultAsync<FilterOptionsResponse, SdkError> {
    return this.client.request<FilterOptionsResponse>((httpClient, headers) =>
      sdk.getApiAdminEventTypesFiltersApplications({
        client: httpClient,
        headers,
      }),
    );
  }

  /**
   * Get distinct subdomains for filtering.
   */
  filterSubdomains(application?: string[]): ResultAsync<FilterOptionsResponse, SdkError> {
    return this.client.request<FilterOptionsResponse>((httpClient, headers) =>
      sdk.getApiAdminEventTypesFiltersSubdomains({
        client: httpClient,
        headers,
        query: { application },
      }),
    );
  }

  /**
   * Get distinct aggregates for filtering.
   */
  filterAggregates(
    application?: string[],
    subdomain?: string[],
  ): ResultAsync<FilterOptionsResponse, SdkError> {
    return this.client.request<FilterOptionsResponse>((httpClient, headers) =>
      sdk.getApiAdminEventTypesFiltersAggregates({
        client: httpClient,
        headers,
        query: { application, subdomain },
      }),
    );
  }

  /**
   * Sync event types for an application.
   */
  sync(
    applicationCode: string,
    eventTypes: PostApiAdminEventTypesSyncData['body']['eventTypes'],
    removeUnlisted = false,
  ): ResultAsync<SyncEventTypesResponse, SdkError> {
    return this.client.request<SyncEventTypesResponse>((httpClient, headers) =>
      sdk.postApiAdminEventTypesSync({
        client: httpClient,
        headers,
        body: { applicationCode, eventTypes, removeUnlisted },
      }),
    );
  }
}
