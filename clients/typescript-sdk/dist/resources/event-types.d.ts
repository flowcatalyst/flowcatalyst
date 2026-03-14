/**
 * Event Types Resource
 *
 * Manage event type definitions and schemas.
 */
import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors";
import type { FlowCatalystClient } from "../client";
import type { GetApiAdminEventTypesResponse, GetApiAdminEventTypesByIdResponse, PostApiAdminEventTypesData, PatchApiAdminEventTypesByIdData, PostApiAdminEventTypesByIdSchemasData, PostApiAdminEventTypesSyncData, PostApiAdminEventTypesSyncResponse, GetApiAdminEventTypesFiltersApplicationsResponse } from "../generated/types.gen";
export type EventTypeListResponse = GetApiAdminEventTypesResponse;
export type EventTypeResponse = GetApiAdminEventTypesByIdResponse;
export type CreateEventTypeRequest = PostApiAdminEventTypesData["body"];
export type UpdateEventTypeRequest = PatchApiAdminEventTypesByIdData["body"];
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
export declare class EventTypesResource {
    private readonly client;
    constructor(client: FlowCatalystClient);
    /**
     * List all event types with optional filters.
     */
    list(filters?: EventTypeFilters): ResultAsync<EventTypeListResponse, SdkError>;
    /**
     * Get an event type by ID.
     */
    get(id: string): ResultAsync<EventTypeResponse, SdkError>;
    /**
     * Create a new event type.
     */
    create(data: CreateEventTypeRequest): ResultAsync<EventTypeResponse, SdkError>;
    /**
     * Update an event type.
     */
    update(id: string, data: UpdateEventTypeRequest): ResultAsync<EventTypeResponse, SdkError>;
    /**
     * Add a schema version to an event type.
     */
    addSchema(id: string, schema: PostApiAdminEventTypesByIdSchemasData["body"]): ResultAsync<EventTypeResponse, SdkError>;
    /**
     * Finalise a schema version (FINALISING -> CURRENT).
     */
    finaliseSchema(id: string, version: string): ResultAsync<EventTypeResponse, SdkError>;
    /**
     * Deprecate a schema version (CURRENT -> DEPRECATED).
     */
    deprecateSchema(id: string, version: string): ResultAsync<EventTypeResponse, SdkError>;
    /**
     * Archive an event type.
     */
    archive(id: string): ResultAsync<EventTypeResponse, SdkError>;
    /**
     * Delete an event type.
     */
    delete(id: string): ResultAsync<unknown, SdkError>;
    /**
     * Get distinct application names for filtering.
     */
    filterApplications(): ResultAsync<FilterOptionsResponse, SdkError>;
    /**
     * Get distinct subdomains for filtering.
     */
    filterSubdomains(application?: string[]): ResultAsync<FilterOptionsResponse, SdkError>;
    /**
     * Get distinct aggregates for filtering.
     */
    filterAggregates(application?: string[], subdomain?: string[]): ResultAsync<FilterOptionsResponse, SdkError>;
    /**
     * Sync event types for an application.
     */
    sync(applicationCode: string, eventTypes: PostApiAdminEventTypesSyncData["body"]["eventTypes"], removeUnlisted?: boolean): ResultAsync<SyncEventTypesResponse, SdkError>;
}
//# sourceMappingURL=event-types.d.ts.map