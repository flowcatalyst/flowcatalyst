/**
 * OIDC Token Manager
 *
 * Handles OAuth2 client credentials flow with token caching.
 */
import { ResultAsync } from "neverthrow";
import type { AuthenticationError } from "./errors";
export interface TokenManagerConfig {
    /** Base URL of the FlowCatalyst platform */
    baseUrl: string;
    /** OAuth client ID */
    clientId: string;
    /** OAuth client secret */
    clientSecret: string;
    /** Optional custom token endpoint (defaults to {baseUrl}/oauth/token) */
    tokenUrl?: string;
}
/**
 * Manages OAuth2 access tokens with automatic refresh.
 */
export declare class OidcTokenManager {
    private cachedToken;
    private refreshPromise;
    private readonly config;
    constructor(config: TokenManagerConfig);
    /**
     * Get a valid access token, fetching a new one if necessary.
     */
    getAccessToken(): ResultAsync<string, AuthenticationError>;
    /**
     * Force refresh the access token.
     */
    refreshToken(): ResultAsync<string, AuthenticationError>;
    /**
     * Check if credentials are configured.
     */
    hasCredentials(): boolean;
    /**
     * Clear the cached token.
     */
    clearCache(): void;
    /**
     * Fetch a new token from the OAuth server.
     */
    private fetchNewToken;
}
//# sourceMappingURL=auth.d.ts.map