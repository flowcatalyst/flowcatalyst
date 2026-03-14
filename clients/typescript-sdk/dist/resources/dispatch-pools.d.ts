/**
 * Dispatch Pools Resource
 *
 * Manage dispatch pools for rate limiting and concurrency control.
 */
import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors";
import type { FlowCatalystClient } from "../client";
import type { GetApiAdminDispatchPoolsResponse, GetApiAdminDispatchPoolsByIdResponse, PostApiAdminDispatchPoolsData, PutApiAdminDispatchPoolsByIdData, PostApiAdminDispatchPoolsSyncData, PostApiAdminDispatchPoolsSyncResponse } from "../generated/types.gen";
export type DispatchPoolListResponse = GetApiAdminDispatchPoolsResponse;
export type DispatchPoolDto = GetApiAdminDispatchPoolsByIdResponse;
export type CreateDispatchPoolRequest = PostApiAdminDispatchPoolsData["body"];
export type UpdateDispatchPoolRequest = PutApiAdminDispatchPoolsByIdData["body"];
export type SyncDispatchPoolsResponse = PostApiAdminDispatchPoolsSyncResponse;
export interface DispatchPoolFilters {
    clientId?: string;
    status?: string;
}
/**
 * Dispatch Pools resource for managing rate limiting and concurrency.
 */
export declare class DispatchPoolsResource {
    private readonly client;
    constructor(client: FlowCatalystClient);
    /**
     * List all dispatch pools with optional filters.
     */
    list(filters?: DispatchPoolFilters): ResultAsync<DispatchPoolListResponse, SdkError>;
    /**
     * Get a dispatch pool by ID.
     */
    get(id: string): ResultAsync<DispatchPoolDto, SdkError>;
    /**
     * Create a new dispatch pool.
     */
    create(data: CreateDispatchPoolRequest): ResultAsync<DispatchPoolDto, SdkError>;
    /**
     * Update a dispatch pool.
     */
    update(id: string, data: UpdateDispatchPoolRequest): ResultAsync<DispatchPoolDto, SdkError>;
    /**
     * Delete (archive) a dispatch pool.
     */
    delete(id: string): ResultAsync<unknown, SdkError>;
    /**
     * Suspend a dispatch pool.
     */
    suspend(id: string): ResultAsync<DispatchPoolDto, SdkError>;
    /**
     * Activate a dispatch pool.
     */
    activate(id: string): ResultAsync<DispatchPoolDto, SdkError>;
    /**
     * Sync dispatch pools for an application.
     */
    sync(applicationCode: string, pools: PostApiAdminDispatchPoolsSyncData["body"]["pools"], removeUnlisted?: boolean): ResultAsync<SyncDispatchPoolsResponse, SdkError>;
}
//# sourceMappingURL=dispatch-pools.d.ts.map