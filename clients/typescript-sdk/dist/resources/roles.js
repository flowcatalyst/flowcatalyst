/**
 * Roles Resource
 *
 * Manage roles and permissions.
 */
import * as sdk from "../generated/sdk.gen";
/**
 * Roles resource for managing role-based access control.
 */
export class RolesResource {
    constructor(client) {
        this.client = client;
    }
    /**
     * List all roles.
     */
    list() {
        return this.client.request((httpClient, headers) => sdk.getApiAdminRoles({
            client: httpClient,
            headers,
        }));
    }
    /**
     * Get a role by name.
     */
    get(roleName) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminRolesByName({
            client: httpClient,
            headers,
            path: { name: roleName },
        }));
    }
    /**
     * Create a new role.
     */
    create(data) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminRoles({
            client: httpClient,
            headers,
            body: data,
        }));
    }
    /**
     * Update a role.
     */
    update(roleName, data) {
        return this.client.request((httpClient, headers) => sdk.putApiAdminRolesByName({
            client: httpClient,
            headers,
            path: { name: roleName },
            body: data,
        }));
    }
    /**
     * Delete a role.
     */
    delete(roleName) {
        return this.client.request((httpClient, headers) => sdk.deleteApiAdminRolesByName({
            client: httpClient,
            headers,
            path: { name: roleName },
        }));
    }
    /**
     * List roles for an application.
     */
    listForApplication(applicationId) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminRolesByApplicationByApplicationId({
            client: httpClient,
            headers,
            path: { applicationId },
        }));
    }
}
