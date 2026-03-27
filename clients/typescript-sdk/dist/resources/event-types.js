/**
 * Event Types Resource
 *
 * Manage event type definitions and schemas.
 */
import * as sdk from "../generated/sdk.gen";
/**
 * Event Types resource for managing event type definitions.
 */
export class EventTypesResource {
    constructor(client) {
        this.client = client;
    }
    /**
     * List all event types with optional filters.
     */
    list(filters, pagination) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminEventTypes({
            client: httpClient,
            headers,
            query: {
                pagination: pagination ?? {},
                ...filters,
            },
        }));
    }
    /**
     * Get an event type by ID.
     */
    get(id) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminEventTypesById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Create a new event type.
     */
    create(data) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminEventTypes({
            client: httpClient,
            headers,
            body: data,
        }));
    }
    /**
     * Update an event type.
     */
    update(id, data) {
        return this.client.request((httpClient, headers) => sdk.putApiAdminEventTypesById({
            client: httpClient,
            headers,
            path: { id },
            body: data,
        }));
    }
    /**
     * Add a schema version to an event type.
     */
    addSchema(id, schema) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminEventTypesByIdSchemas({
            client: httpClient,
            headers,
            path: { id },
            body: schema,
        }));
    }
    /**
     * Delete an event type.
     */
    delete(id) {
        return this.client.request((httpClient, headers) => sdk.deleteApiAdminEventTypesById({
            client: httpClient,
            headers,
            path: { id },
        }));
    }
    /**
     * Get distinct application names for filtering.
     */
    filterApplications() {
        return this.client.request((httpClient, headers) => sdk.getApiAdminFilterOptionsEventTypesFiltersApplications({
            client: httpClient,
            headers,
        }));
    }
    /**
     * Get distinct subdomains for filtering.
     */
    filterSubdomains(application) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminFilterOptionsEventTypesFiltersSubdomains({
            client: httpClient,
            headers,
            query: { "application[]": application },
        }));
    }
    /**
     * Get distinct aggregates for filtering.
     */
    filterAggregates(application, subdomain) {
        return this.client.request((httpClient, headers) => sdk.getApiAdminFilterOptionsEventTypesFiltersAggregates({
            client: httpClient,
            headers,
            query: { "application[]": application, "subdomain[]": subdomain },
        }));
    }
    /**
     * Sync event types for an application.
     */
    sync(applicationCode, eventTypes, removeUnlisted = false) {
        return this.client.request((httpClient, headers) => sdk.postApiAdminEventTypesSync({
            client: httpClient,
            headers,
            body: { applicationCode, eventTypes },
            query: { removeUnlisted },
        }));
    }
}
