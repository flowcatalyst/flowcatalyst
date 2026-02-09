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
  ApplicationResponse,
  ApplicationListResponse,
  CreateApplicationRequest,
  UpdateApplicationRequest,
  CreateServiceAccountResponse,
} from '../generated/types.gen';

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
      sdk.listApplications({
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
      sdk.getApplication({
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
      sdk.getApplicationByCode({
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
      sdk.createApplication({
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
      sdk.updateApplication({
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
      sdk.deleteApplication({
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
      sdk.activateApplication({
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
      sdk.deactivateApplication({
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
      sdk.provisionApplicationServiceAccount({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }
}
