/**
 * Clients Resource
 *
 * Manage clients (tenants) in the platform.
 */

import type { ResultAsync } from 'neverthrow';
import type { SdkError } from '../errors';
import type { FlowCatalystClient } from '../client';
import * as sdk from '../generated/sdk.gen';
import type {
  ClientDto,
  ClientListResponse,
  ClientApplicationsResponse,
  UpdateClientApplicationsRequest,
  CreateClientRequest,
  UpdateClientRequest,
} from '../generated/types.gen';

/**
 * Response for status change operations (enable/disable).
 */
export interface StatusResponse {
  message: string;
}

/**
 * Clients resource for managing platform clients (tenants).
 */
export class ClientsResource {
  constructor(private readonly client: FlowCatalystClient) {}

  /**
   * List all clients.
   */
  list(): ResultAsync<ClientListResponse, SdkError> {
    return this.client.request<ClientListResponse>((httpClient, headers) =>
      sdk.listClients({
        client: httpClient,
        headers,
      }),
    );
  }

  /**
   * Get a client by ID.
   */
  get(id: string): ResultAsync<ClientDto, SdkError> {
    return this.client.request<ClientDto>((httpClient, headers) =>
      sdk.getClient({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Get a client by identifier.
   */
  getByIdentifier(identifier: string): ResultAsync<ClientDto, SdkError> {
    return this.client.request<ClientDto>((httpClient, headers) =>
      sdk.getClientByIdentifier({
        client: httpClient,
        headers,
        path: { identifier },
      }),
    );
  }

  /**
   * Create a new client.
   */
  create(data: CreateClientRequest): ResultAsync<ClientDto, SdkError> {
    return this.client.request<ClientDto>((httpClient, headers) =>
      sdk.createClient({
        client: httpClient,
        headers,
        body: data,
      }),
    );
  }

  /**
   * Update a client.
   */
  update(id: string, data: UpdateClientRequest): ResultAsync<ClientDto, SdkError> {
    return this.client.request<ClientDto>((httpClient, headers) =>
      sdk.updateClient({
        client: httpClient,
        headers,
        path: { id },
        body: data,
      }),
    );
  }

  /**
   * Activate a client.
   */
  activate(id: string): ResultAsync<ClientDto, SdkError> {
    return this.client.request<ClientDto>((httpClient, headers) =>
      sdk.activateClient({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Deactivate a client.
   */
  deactivate(id: string, reason: string): ResultAsync<ClientDto, SdkError> {
    return this.client.request<ClientDto>((httpClient, headers) =>
      sdk.deactivateClient({
        client: httpClient,
        headers,
        path: { id },
        body: { reason },
      }),
    );
  }

  /**
   * Suspend a client with a reason.
   */
  suspend(id: string, reason: string): ResultAsync<ClientDto, SdkError> {
    return this.client.request<ClientDto>((httpClient, headers) =>
      sdk.suspendClient({
        client: httpClient,
        headers,
        path: { id },
        body: { reason },
      }),
    );
  }

  /**
   * Get applications configured for a client.
   */
  getApplications(id: string): ResultAsync<ClientApplicationsResponse, SdkError> {
    return this.client.request<ClientApplicationsResponse>((httpClient, headers) =>
      sdk.getClientApplications({
        client: httpClient,
        headers,
        path: { id },
      }),
    );
  }

  /**
   * Update the applications configured for a client.
   */
  updateApplications(
    id: string,
    data: UpdateClientApplicationsRequest,
  ): ResultAsync<ClientApplicationsResponse, SdkError> {
    return this.client.request<ClientApplicationsResponse>((httpClient, headers) =>
      sdk.updateClientApplications({
        client: httpClient,
        headers,
        path: { id },
        body: data,
      }),
    );
  }

  /**
   * Enable an application for a client.
   */
  enableApplication(
    clientId: string,
    applicationId: string,
  ): ResultAsync<StatusResponse, SdkError> {
    return this.client.request<StatusResponse>((httpClient, headers) =>
      sdk.enableClientApplication({
        client: httpClient,
        headers,
        path: { id: clientId, applicationId },
      }),
    );
  }

  /**
   * Disable an application for a client.
   */
  disableApplication(
    clientId: string,
    applicationId: string,
  ): ResultAsync<StatusResponse, SdkError> {
    return this.client.request<StatusResponse>((httpClient, headers) =>
      sdk.disableClientApplication({
        client: httpClient,
        headers,
        path: { id: clientId, applicationId },
      }),
    );
  }
}
