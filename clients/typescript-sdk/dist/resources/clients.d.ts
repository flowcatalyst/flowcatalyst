/**
 * Clients Resource
 *
 * Manage clients (tenants) in the platform.
 */
import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors";
import type { FlowCatalystClient } from "../client";
import type { GetApiAdminClientsResponse, GetApiAdminClientsByIdResponse, GetApiAdminClientsByIdApplicationsResponse, PutApiAdminClientsByIdApplicationsData, PostApiAdminClientsData, PutApiAdminClientsByIdData } from "../generated/types.gen";
export type ClientListResponse = GetApiAdminClientsResponse;
export type ClientDto = GetApiAdminClientsByIdResponse;
export type ClientApplicationsResponse = GetApiAdminClientsByIdApplicationsResponse;
export type CreateClientRequest = PostApiAdminClientsData["body"];
export type UpdateClientRequest = PutApiAdminClientsByIdData["body"];
export type UpdateClientApplicationsRequest = PutApiAdminClientsByIdApplicationsData["body"];
/**
 * Response for status change operations (enable/disable).
 */
export interface StatusResponse {
    message: string;
}
/**
 * Clients resource for managing platform clients (tenants).
 */
export declare class ClientsResource {
    private readonly client;
    constructor(client: FlowCatalystClient);
    /**
     * List all clients.
     */
    list(): ResultAsync<ClientListResponse, SdkError>;
    /**
     * Get a client by ID.
     */
    get(id: string): ResultAsync<ClientDto, SdkError>;
    /**
     * Get a client by identifier.
     */
    getByIdentifier(identifier: string): ResultAsync<ClientDto, SdkError>;
    /**
     * Create a new client.
     */
    create(data: CreateClientRequest): ResultAsync<ClientDto, SdkError>;
    /**
     * Update a client.
     */
    update(id: string, data: UpdateClientRequest): ResultAsync<ClientDto, SdkError>;
    /**
     * Activate a client.
     */
    activate(id: string): ResultAsync<ClientDto, SdkError>;
    /**
     * Deactivate a client.
     */
    deactivate(id: string, reason: string): ResultAsync<ClientDto, SdkError>;
    /**
     * Suspend a client with a reason.
     */
    suspend(id: string, reason: string): ResultAsync<ClientDto, SdkError>;
    /**
     * Get applications configured for a client.
     */
    getApplications(id: string): ResultAsync<ClientApplicationsResponse, SdkError>;
    /**
     * Update the applications configured for a client.
     */
    updateApplications(id: string, data: UpdateClientApplicationsRequest): ResultAsync<ClientApplicationsResponse, SdkError>;
    /**
     * Enable an application for a client.
     */
    enableApplication(clientId: string, applicationId: string): ResultAsync<StatusResponse, SdkError>;
    /**
     * Disable an application for a client.
     */
    disableApplication(clientId: string, applicationId: string): ResultAsync<StatusResponse, SdkError>;
}
//# sourceMappingURL=clients.d.ts.map