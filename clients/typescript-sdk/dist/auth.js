/**
 * OIDC Token Manager
 *
 * Handles OAuth2 client credentials flow with token caching.
 */
import { ResultAsync } from "neverthrow";
import { authError } from "./errors";
/**
 * Manages OAuth2 access tokens with automatic refresh.
 */
export class OidcTokenManager {
    constructor(config) {
        this.cachedToken = null;
        this.refreshPromise = null;
        this.config = config;
    }
    /**
     * Get a valid access token, fetching a new one if necessary.
     */
    getAccessToken() {
        // Check if we have a valid cached token (with 60s buffer)
        if (this.cachedToken && this.cachedToken.expiresAt > Date.now() + 60000) {
            return ResultAsync.fromSafePromise(Promise.resolve(this.cachedToken.token));
        }
        // Prevent concurrent token fetches
        if (this.refreshPromise) {
            return ResultAsync.fromPromise(this.refreshPromise, (e) => authError.tokenFetchFailed("Token fetch failed", e));
        }
        return this.fetchNewToken();
    }
    /**
     * Force refresh the access token.
     */
    refreshToken() {
        this.cachedToken = null;
        return this.fetchNewToken();
    }
    /**
     * Check if credentials are configured.
     */
    hasCredentials() {
        return !!(this.config.clientId && this.config.clientSecret);
    }
    /**
     * Clear the cached token.
     */
    clearCache() {
        this.cachedToken = null;
        this.refreshPromise = null;
    }
    /**
     * Fetch a new token from the OAuth server.
     */
    fetchNewToken() {
        if (!this.hasCredentials()) {
            return ResultAsync.fromSafePromise(Promise.reject(authError.missingCredentials())).mapErr(() => authError.missingCredentials());
        }
        const tokenUrl = this.config.tokenUrl ??
            `${this.config.baseUrl.replace(/\/$/, "")}/oauth/token`;
        const fetchPromise = fetch(tokenUrl, {
            method: "POST",
            headers: {
                "Content-Type": "application/x-www-form-urlencoded",
                Accept: "application/json",
            },
            body: new URLSearchParams({
                grant_type: "client_credentials",
                client_id: this.config.clientId,
                client_secret: this.config.clientSecret,
            }),
        });
        this.refreshPromise = fetchPromise.then(async (response) => {
            this.refreshPromise = null;
            if (response.status === 401 || response.status === 403) {
                throw authError.invalidCredentials();
            }
            if (!response.ok) {
                const body = await response.json().catch(() => ({}));
                const errorMsg = body["error_description"]?.toString() ??
                    body["error"]?.toString() ??
                    "Token fetch failed";
                throw authError.tokenFetchFailed(errorMsg);
            }
            const data = (await response.json());
            if (!data.access_token) {
                throw authError.tokenFetchFailed("No access token in response");
            }
            // Cache with expiry
            this.cachedToken = {
                token: data.access_token,
                expiresAt: Date.now() + data.expires_in * 1000,
            };
            return data.access_token;
        });
        return ResultAsync.fromPromise(this.refreshPromise, (e) => {
            this.refreshPromise = null;
            if (e.type) {
                return e;
            }
            return authError.tokenFetchFailed(e instanceof Error ? e.message : "Unknown error", e instanceof Error ? e : undefined);
        });
    }
}
