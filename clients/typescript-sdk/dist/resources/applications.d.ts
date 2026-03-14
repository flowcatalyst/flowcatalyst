/**
 * Applications Resource
 *
 * Manage applications in the platform.
 */
import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors";
import type { FlowCatalystClient } from "../client";
import type { GetApiAdminApplicationsResponse, GetApiAdminApplicationsByIdResponse, PostApiAdminApplicationsData, PutApiAdminApplicationsByIdData, PostApiAdminApplicationsByIdProvisionServiceAccountResponse } from "../generated/types.gen";
export type ApplicationListResponse = GetApiAdminApplicationsResponse;
export type ApplicationResponse = GetApiAdminApplicationsByIdResponse;
export type CreateApplicationRequest = PostApiAdminApplicationsData["body"];
export type UpdateApplicationRequest = PutApiAdminApplicationsByIdData["body"];
export type CreateServiceAccountResponse = PostApiAdminApplicationsByIdProvisionServiceAccountResponse;
/**
 * Applications resource for managing platform applications.
 */
export declare class ApplicationsResource {
    private readonly client;
    constructor(client: FlowCatalystClient);
    /**
     * List all applications.
     */
    list(): ResultAsync<ApplicationListResponse, SdkError>;
    /**
     * Get an application by ID.
     */
    get(id: string): ResultAsync<ApplicationResponse, SdkError>;
    /**
     * Get an application by code.
     */
    getByCode(code: string): ResultAsync<ApplicationResponse, SdkError>;
    /**
     * Create a new application.
     */
    create(data: CreateApplicationRequest): ResultAsync<ApplicationResponse, SdkError>;
    /**
     * Update an application.
     */
    update(id: string, data: UpdateApplicationRequest): ResultAsync<ApplicationResponse, SdkError>;
    /**
     * Delete an application.
     */
    delete(id: string): ResultAsync<unknown, SdkError>;
    /**
     * Activate an application.
     */
    activate(id: string): ResultAsync<ApplicationResponse, SdkError>;
    /**
     * Deactivate an application.
     */
    deactivate(id: string): ResultAsync<ApplicationResponse, SdkError>;
    /**
     * Provision a service account for an application.
     */
    provisionServiceAccount(id: string): ResultAsync<CreateServiceAccountResponse, SdkError>;
}
//# sourceMappingURL=applications.d.ts.map