/**
 * Subscriptions Resource
 *
 * Manage event subscriptions for webhook delivery.
 */
import * as sdk from "../generated/sdk.gen";
/**
 * Subscriptions resource for managing event subscriptions.
 */
export class SubscriptionsResource {
    constructor(client) {
        this.client = client;
    }
    /**
     * List all subscriptions with optional filters.
     */
    list(filters) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminSubscriptions({
            client: httpClient,
            headers,
            query: filters,
        }));
    }
    /**
     * Get a subscription by ID.
     */
    get(id) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminSubscriptionsById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Create a new subscription.
     */
    create(data) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminSubscriptions({
            client: httpClient,
            headers,
            body: data,
        }));
    }
    /**
     * Update a subscription.
     */
    update(id, data) {
        return this.client.request((httpClient, headers) => sdk.putApiAdminSubscriptionsById({
            client: httpClient,
            headers,
            path: { id },
            body: data,
        }));
    }
    /**
     * Delete a subscription.
     */
    delete(id) {
        return this.client.request((httpClient, headers) => sdk.deleteApiAdminSubscriptionsById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Pause a subscription.
     */
    pause(id) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminSubscriptionsByIdPause({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Resume a paused subscription.
     */
    resume(id) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminSubscriptionsByIdResume({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Sync subscriptions for an application.
     */
    sync(applicationCode, subscriptions, removeUnlisted = false) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminSubscriptionsSync({
            client: httpClient,
            headers,
            body: { applicationCode, subscriptions, removeUnlisted },
        }));
    }
}
