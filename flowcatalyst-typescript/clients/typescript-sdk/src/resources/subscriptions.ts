/**
 * Subscriptions Resource
 *
 * Manage event subscriptions for webhook delivery.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type {
  GetApiAdminSubscriptionsResponse,
  GetApiAdminSubscriptionsByIdResponse,
  PostApiAdminSubscriptionsData,
  PutApiAdminSubscriptionsByIdData,
  PostApiAdminSubscriptionsSyncData,
  PostApiAdminSubscriptionsSyncResponse,
} from '../generated/types.gen';

export type SubscriptionListResponse = GetApiAdminSubscriptionsResponse;
export type SubscriptionDto = GetApiAdminSubscriptionsByIdResponse;
export type CreateSubscriptionRequest = PostApiAdminSubscriptionsData['body'];
export type UpdateSubscriptionRequest = PutApiAdminSubscriptionsByIdData['body'];
export type SyncSubscriptionsResponse = PostApiAdminSubscriptionsSyncResponse;

export interface SubscriptionFilters {
  clientId?: string;
  status?: string;
  dispatchPoolId?: string;
  source?: string;
  anchorLevel?: string;
}

/**
 * Subscriptions resource for managing event subscriptions.
 */
export class SubscriptionsResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all subscriptions with optional filters.
   */
  list(filters?: SubscriptionFilters): ResultAsync<SubscriptionListResponse, SdkError> {
    return this.client.request<SubscriptionListResponse>((httpClient, headers) =>
      sdk.getApiAdminSubscriptions({
        client: httpClient,
        headers,
        query: filters,
      }),
    );
  }

  /**
   * Get a subscription by ID.
   */
  get(id: string): ResultAsync<SubscriptionDto, SdkError> {
    return this.client.request<SubscriptionDto>((httpClient, headers) =>
      sdk.getApiAdminSubscriptionsById({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Create a new subscription.
   */
  create(data: CreateSubscriptionRequest): ResultAsync<SubscriptionDto, SdkError> {
    return this.client.request<SubscriptionDto>((httpClient, headers) =>
      sdk.postApiAdminSubscriptions({
        client: httpClient,
        headers,
        body: data,
      }),
    );
  }

  /**
   * Update a subscription.
   */
  update(id: string, data: UpdateSubscriptionRequest): ResultAsync<SubscriptionDto, SdkError> {
    return this.client.request<SubscriptionDto>((httpClient, headers) =>
      sdk.putApiAdminSubscriptionsById({
        client: httpClient,
        headers,
        path: { id },
        body: data,
      }),
    );
  }

  /**
   * Delete a subscription.
   */
  delete(id: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.deleteApiAdminSubscriptionsById({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Pause a subscription.
   */
  pause(id: string): ResultAsync<SubscriptionDto, SdkError> {
    return this.client.request<SubscriptionDto>((httpClient, headers) =>
      sdk.postApiAdminSubscriptionsByIdPause({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Resume a paused subscription.
   */
  resume(id: string): ResultAsync<SubscriptionDto, SdkError> {
    return this.client.request<SubscriptionDto>((httpClient, headers) =>
      sdk.postApiAdminSubscriptionsByIdResume({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Sync subscriptions for an application.
   */
  sync(
    applicationCode: string,
    subscriptions: PostApiAdminSubscriptionsSyncData['body']['subscriptions'],
    removeUnlisted = false,
  ): ResultAsync<SyncSubscriptionsResponse, SdkError> {
    return this.client.request<SyncSubscriptionsResponse>((httpClient, headers) =>
      sdk.postApiAdminSubscriptionsSync({
        client: httpClient,
        headers,
        body: { applicationCode, subscriptions, removeUnlisted },
      }),
    );
  }
}
