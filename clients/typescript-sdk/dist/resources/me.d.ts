/**
 * Me Resource
 *
 * User-scoped endpoints for accessing resources the authenticated user has access to.
 * These endpoints do NOT require admin permissions.
 */
import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors";
import type { FlowCatalystClient } from "../client";
/**
 * Client the user has access to.
 */
export interface MyClient {
    id: string;
    name: string;
    identifier: string;
    status: "ACTIVE" | "SUSPENDED" | "INACTIVE";
    createdAt: string;
    updatedAt: string;
}
/**
 * Response for listing accessible clients.
 */
export interface MyClientsResponse {
    clients: MyClient[];
    total: number;
}
/**
 * Application enabled for a client.
 */
export interface MyApplication {
    id: string;
    code: string;
    name: string;
    description: string | null;
    iconUrl: string | null;
    baseUrl: string | null;
    website: string | null;
    logoMimeType: string | null;
}
/**
 * Response for listing applications for a client.
 */
export interface MyApplicationsResponse {
    applications: MyApplication[];
    total: number;
    clientId: string;
}
/**
 * Me resource for user-scoped access to clients and applications.
 */
export declare class MeResource {
    private readonly client;
    constructor(client: FlowCatalystClient);
    /**
     * Get clients the authenticated user has access to.
     *
     * Access is determined by user scope:
     * - ANCHOR: All active clients
     * - PARTNER: IDP granted clients + explicit grants
     * - CLIENT: Home client + IDP additional clients + explicit grants
     */
    getClients(): ResultAsync<MyClientsResponse, SdkError>;
    /**
     * Get a specific client the user has access to.
     */
    getClient(clientId: string): ResultAsync<MyClient, SdkError>;
    /**
     * Get applications enabled for a client the user has access to.
     */
    getClientApplications(clientId: string): ResultAsync<MyApplicationsResponse, SdkError>;
}
//# sourceMappingURL=me.d.ts.map