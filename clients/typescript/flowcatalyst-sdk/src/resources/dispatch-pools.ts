/**
 * Dispatch Pools Resource
 *
 * Manage dispatch pools for rate limiting and concurrency control.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type {
  DispatchPoolDto,
  DispatchPoolListResponse,
  CreateDispatchPoolRequest,
  UpdateDispatchPoolRequest,
  SyncPoolsRequest,
  SyncResponse,
  PoolListResponse,
  DispatchPoolStatus,
} from '../generated/types.gen';

export interface DispatchPoolFilters {
  clientId?: string;
  status?: DispatchPoolStatus;
}

/**
 * Dispatch Pools resource for managing rate limiting and concurrency.
 */
export class DispatchPoolsResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all dispatch pools with optional filters.
   */
  list(filters?: DispatchPoolFilters): ResultAsync<DispatchPoolListResponse, SdkError> {
    return this.client.request<DispatchPoolListResponse>((httpClient, headers) =>
      sdk.getApiAdminDispatchPools({
        client: httpClient,
        headers,
        query: filters,
      })
    );
  }

  /**
   * Get a dispatch pool by ID.
   */
  get(id: string): ResultAsync<DispatchPoolDto, SdkError> {
    return this.client.request<DispatchPoolDto>((httpClient, headers) =>
      sdk.getApiAdminDispatchPoolsById({
        client: httpClient,
        headers,
        path: { id },
      })
    );
  }

  /**
   * Create a new dispatch pool.
   */
  create(data: CreateDispatchPoolRequest): ResultAsync<DispatchPoolDto, SdkError> {
    return this.client.request<DispatchPoolDto>((httpClient, headers) =>
      sdk.postApiAdminDispatchPools({
        client: httpClient,
        headers,
        body: data,
      })
    );
  }

  /**
   * Update a dispatch pool.
   */
  update(id: string, data: UpdateDispatchPoolRequest): ResultAsync<DispatchPoolDto, SdkError> {
    return this.client.request<DispatchPoolDto>((httpClient, headers) =>
      sdk.putApiAdminDispatchPoolsById({
        client: httpClient,
        headers,
        path: { id },
        body: data,
      })
    );
  }

  /**
   * Delete (archive) a dispatch pool.
   */
  delete(id: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.deleteApiAdminDispatchPoolsById({
        client: httpClient,
        headers,
        path: { id },
      })
    );
  }

  /**
   * Suspend a dispatch pool.
   */
  suspend(id: string): ResultAsync<DispatchPoolDto, SdkError> {
    return this.client.request<DispatchPoolDto>((httpClient, headers) =>
      sdk.postApiAdminDispatchPoolsByIdSuspend({
        client: httpClient,
        headers,
        path: { id },
      })
    );
  }

  /**
   * Activate a dispatch pool.
   */
  activate(id: string): ResultAsync<DispatchPoolDto, SdkError> {
    return this.client.request<DispatchPoolDto>((httpClient, headers) =>
      sdk.postApiAdminDispatchPoolsByIdActivate({
        client: httpClient,
        headers,
        path: { id },
      })
    );
  }

  /**
   * List dispatch pools for an application.
   */
  listForApplication(appCode: string): ResultAsync<PoolListResponse, SdkError> {
    return this.client.request<PoolListResponse>((httpClient, headers) =>
      sdk.listApplicationDispatchPools({
        client: httpClient,
        headers,
        path: { appCode },
      })
    );
  }

  /**
   * Sync dispatch pools for an application.
   */
  sync(
    appCode: string,
    pools: SyncPoolsRequest['pools'],
    removeUnlisted = false
  ): ResultAsync<SyncResponse, SdkError> {
    return this.client.request<SyncResponse>((httpClient, headers) =>
      sdk.syncApplicationDispatchPools({
        client: httpClient,
        headers,
        path: { appCode },
        query: { removeUnlisted },
        body: { pools },
      })
    );
  }
}
