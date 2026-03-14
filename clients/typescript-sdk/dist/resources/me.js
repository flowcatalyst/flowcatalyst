/**
 * Me Resource
 *
 * User-scoped endpoints for accessing resources the authenticated user has access to.
 * These endpoints do NOT require admin permissions.
 */
/**
 * Me resource for user-scoped access to clients and applications.
 */
export class MeResource {
    constructor(client) {
        this.client = client;
    }
    /**
     * Get clients the authenticated user has access to.
     *
     * Access is determined by user scope:
     * - ANCHOR: All active clients
     * - PARTNER: IDP granted clients + explicit grants
     * - CLIENT: Home client + IDP additional clients + explicit grants
     */
    getClients() {
        return this.client.request((httpClient, headers) => httpClient.get({
            url: "/api/me/clients",
            headers,
        }));
    }
    /**
     * Get a specific client the user has access to.
     */
    getClient(clientId) {
        return this.client.request((httpClient, headers) => httpClient.get({
            url: "/api/me/clients/{clientId}",
            path: { clientId },
            headers,
        }));
    }
    /**
     * Get applications enabled for a client the user has access to.
     */
    getClientApplications(clientId) {
        return this.client.request((httpClient, headers) => httpClient.get({
            url: "/api/me/clients/{clientId}/applications",
            path: { clientId },
            headers,
        }));
    }
}
