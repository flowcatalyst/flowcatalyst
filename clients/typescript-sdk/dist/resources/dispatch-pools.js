/**
 * Dispatch Pools Resource
 *
 * Manage dispatch pools for rate limiting and concurrency control.
 */
import * as sdk from "../generated/sdk.gen";
/**
 * Dispatch Pools resource for managing rate limiting and concurrency.
 */
export class DispatchPoolsResource {
    constructor(client) {
        this.client = client;
    }
    /**
     * List all dispatch pools with optional filters.
     */
    list(filters) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminDispatchPools({
            client: httpClient,
            headers,
            query: filters,
        }));
    }
    /**
     * Get a dispatch pool by ID.
     */
    get(id) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminDispatchPoolsById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Create a new dispatch pool.
     */
    create(data) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminDispatchPools({
            client: httpClient,
            headers,
            body: data,
        }));
    }
    /**
     * Update a dispatch pool.
     */
    update(id, data) {
        return this.client.request((httpClient, headers) => sdk.putApiAdminDispatchPoolsById({
            client: httpClient,
            headers,
            path: { id },
            body: data,
        }));
    }
    /**
     * Delete (archive) a dispatch pool.
     */
    delete(id) {
        return this.client.request((httpClient, headers) => sdk.deleteApiAdminDispatchPoolsById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Suspend a dispatch pool.
     */
    suspend(id) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminDispatchPoolsByIdSuspend({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Activate a dispatch pool.
     */
    activate(id) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminDispatchPoolsByIdActivate({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Sync dispatch pools for an application.
     */
    sync(applicationCode, pools, removeUnlisted = false) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminDispatchPoolsSync({
            client: httpClient,
            headers,
            body: { applicationCode, pools, removeUnlisted },
        }));
    }
}
