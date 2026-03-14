/**
 * Applications Resource
 *
 * Manage applications in the platform.
 */
import * as sdk from "../generated/sdk.gen";
/**
 * Applications resource for managing platform applications.
 */
export class ApplicationsResource {
    constructor(client) {
        this.client = client;
    }
    /**
     * List all applications.
     */
    list() {
        return this.client.request((httpClient, headers) => sdk.getApiAdminApplications({
            client: httpClient,
            headers,
        }));
    }
    /**
     * Get an application by ID.
     */
    get(id) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminApplicationsById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Get an application by code.
     */
    getByCode(code) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminApplicationsByCodeByCode({
            client: httpClient,
            headers,
            path: { code },
        }));
    }
    /**
     * Create a new application.
     */
    create(data) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminApplications({
            client: httpClient,
            headers,
            body: data,
        }));
    }
    /**
     * Update an application.
     */
    update(id, data) {
        return this.client.request((httpClient, headers) => sdk.putApiAdminApplicationsById({
            client: httpClient,
            headers,
            path: { id },
            body: data,
        }));
    }
    /**
     * Delete an application.
     */
    delete(id) {
        return this.client.request((httpClient, headers) => sdk.deleteApiAdminApplicationsById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Activate an application.
     */
    activate(id) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminApplicationsByIdActivate({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Deactivate an application.
     */
    deactivate(id) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminApplicationsByIdDeactivate({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Provision a service account for an application.
     */
    provisionServiceAccount(id) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminApplicationsByIdProvisionServiceAccount({
            client: httpClient,
            headers,
            path: { id },
            body: {},
        }));
    }
}
