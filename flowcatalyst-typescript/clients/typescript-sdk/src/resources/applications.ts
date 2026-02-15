/**
 * Applications Resource
 *
 * Manage applications in the platform.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type {
  GetApiAdminApplicationsResponse,
  GetApiAdminApplicationsByIdResponse,
  PostApiAdminApplicationsData,
  PutApiAdminApplicationsByIdData,
  PostApiAdminApplicationsByIdProvisionServiceAccountResponse,
} from '../generated/types.gen';

export type ApplicationListResponse = GetApiAdminApplicationsResponse;
export type ApplicationResponse = GetApiAdminApplicationsByIdResponse;
export type CreateApplicationRequest = PostApiAdminApplicationsData['body'];
export type UpdateApplicationRequest = PutApiAdminApplicationsByIdData['body'];
export type CreateServiceAccountResponse = PostApiAdminApplicationsByIdProvisionServiceAccountResponse;

/**
 * Applications resource for managing platform applications.
 */
export class ApplicationsResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all applications.
   */
  list(): ResultAsync<ApplicationListResponse, SdkError> {
    return this.client.request<ApplicationListResponse>((httpClient, headers) =>
      sdk.getApiAdminApplications({
        client: httpClient,
        headers,
      }),
    );
  }

  /**
   * Get an application by ID.
   */
  get(id: string): ResultAsync<ApplicationResponse, SdkError> {
    return this.client.request<ApplicationResponse>((httpClient, headers) =>
      sdk.getApiAdminApplicationsById({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Get an application by code.
   */
  getByCode(code: string): ResultAsync<ApplicationResponse, SdkError> {
    return this.client.request<ApplicationResponse>((httpClient, headers) =>
      sdk.getApiAdminApplicationsByCodeByCode({
        client: httpClient,
        headers,
        path: { code },
      }),
    );
  }

  /**
   * Create a new application.
   */
  create(data: CreateApplicationRequest): ResultAsync<ApplicationResponse, SdkError> {
    return this.client.request<ApplicationResponse>((httpClient, headers) =>
      sdk.postApiAdminApplications({
        client: httpClient,
        headers,
        body: data,
      }),
    );
  }

  /**
   * Update an application.
   */
  update(id: string, data: UpdateApplicationRequest): ResultAsync<ApplicationResponse, SdkError> {
    return this.client.request<ApplicationResponse>((httpClient, headers) =>
      sdk.putApiAdminApplicationsById({
        client: httpClient,
        headers,
        path: { id },
        body: data,
      }),
    );
  }

  /**
   * Delete an application.
   */
  delete(id: string): ResultAsync<unknown, SdkError> {
    return this.client.request<unknown>((httpClient, headers) =>
      sdk.deleteApiAdminApplicationsById({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Activate an application.
   */
  activate(id: string): ResultAsync<ApplicationResponse, SdkError> {
    return this.client.request<ApplicationResponse>((httpClient, headers) =>
      sdk.postApiAdminApplicationsByIdActivate({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Deactivate an application.
   */
  deactivate(id: string): ResultAsync<ApplicationResponse, SdkError> {
    return this.client.request<ApplicationResponse>((httpClient, headers) =>
      sdk.postApiAdminApplicationsByIdDeactivate({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Provision a service account for an application.
   */
  provisionServiceAccount(id: string): ResultAsync<CreateServiceAccountResponse, SdkError> {
    return this.client.request<CreateServiceAccountResponse>((httpClient, headers) =>
      sdk.postApiAdminApplicationsByIdProvisionServiceAccount({
        client: httpClient,
        headers,
        path: { id },
        body: {},
      }),
    );
  }
}
