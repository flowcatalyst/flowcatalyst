/**
 * Clients Resource
 *
 * Manage clients (tenants) in the platform.
 */
import * as sdk from "../generated/sdk.gen";
/**
 * Clients resource for managing platform clients (tenants).
 */
export class ClientsResource {
    constructor(client) {
        this.client = client;
    }
    /**
     * List all clients.
     */
    list() {
        return this.client.request((httpClient, headers) => sdk.getApiAdminClients({
            client: httpClient,
            headers,
        }));
    }
    /**
     * Get a client by ID.
     */
    get(id) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminClientsById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Get a client by identifier.
     */
    getByIdentifier(identifier) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminClientsByIdentifierByIdentifier({
            client: httpClient,
            headers,
            path: { identifier },
        }));
    }
    /**
     * Create a new client.
     */
    create(data) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminClients({
            client: httpClient,
            headers,
            body: data,
        }));
    }
    /**
     * Update a client.
     */
    update(id, data) {
        return this.client.request((httpClient, headers) => sdk.putApiAdminClientsById({
            client: httpClient,
            headers,
            path: { id },
            body: data,
        }));
    }
    /**
     * Activate a client.
     */
    activate(id) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminClientsByIdActivate({
            client: httpClient,
            headers,
            path: { id },
            body: {},
        }));
    }
    /**
     * Deactivate a client.
     */
    deactivate(id, reason) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminClientsByIdDeactivate({
            client: httpClient,
            headers,
            path: { id },
            body: { reason },
        }));
    }
    /**
     * Suspend a client with a reason.
     */
    suspend(id, reason) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminClientsByIdSuspend({
            client: httpClient,
            headers,
            path: { id },
            body: { reason },
        }));
    }
    /**
     * Get applications configured for a client.
     */
    getApplications(id) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminClientsByIdApplications({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Update the applications configured for a client.
     */
    updateApplications(id, data) {
        return this.client.request((httpClient, headers) => sdk.putApiAdminClientsByIdApplications({
            client: httpClient,
            headers,
            path: { id },
            body: data,
        }));
    }
    /**
     * Enable an application for a client.
     */
    enableApplication(clientId, applicationId) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminClientsByIdApplicationsByAppIdEnable({
            client: httpClient,
            headers,
            path: { id: clientId, appId: applicationId },
        }));
    }
    /**
     * Disable an application for a client.
     */
    disableApplication(clientId, applicationId) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminClientsByIdApplicationsByAppIdDisable({
            client: httpClient,
            headers,
            path: { id: clientId, appId: applicationId },
        }));
    }
}
