/**
 * OAuth2 Token Manager
 *
 * Handles client_credentials flow with token caching.
 * Based on the pattern from @flowcatalyst/sdk.
 */

import type { Config } from "./config.js";

interface TokenResponse {
	access_token: string;
	token_type: string;
	expires_in: number;
}

interface CachedToken {
	token: string;
	expiresAt: number;
}

export class TokenManager {
	private cachedToken: CachedToken | null = null;
	private refreshPromise: Promise<string> | null = null;
	private readonly tokenUrl: string;

	constructor(private readonly config: Config) {
		this.tokenUrl = `${config.baseUrl}/oidc/token`;
	}

	/**
	 * Get a valid access token, fetching a new one if necessary.
	 */
	async getAccessToken(): Promise<string> {
		// Check cached token (with 60s buffer)
		if (this.cachedToken && this.cachedToken.expiresAt > Date.now() + 60_000) {
			return this.cachedToken.token;
		}

		// Prevent concurrent token fetches
		if (this.refreshPromise) {
			return this.refreshPromise;
		}

		return this.fetchNewToken();
	}

	/**
	 * Clear cached token to force refresh on next call.
	 */
	clearCache(): void {
		this.cachedToken = null;
		this.refreshPromise = null;
	}

	private async fetchNewToken(): Promise<string> {
		this.refreshPromise = this.doFetch();

		try {
			const token = await this.refreshPromise;
			return token;
		} finally {
			this.refreshPromise = null;
		}
	}

	private async doFetch(): Promise<string> {
		const response = await fetch(this.tokenUrl, {
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

		if (!response.ok) {
			const body = await response.text().catch(() => "");
			throw new Error(
				`Token fetch failed (${response.status}): ${body}`,
			);
		}

		const data = (await response.json()) as TokenResponse;
		if (!data.access_token) {
			throw new Error("No access_token in token response");
		}

		this.cachedToken = {
			token: data.access_token,
			expiresAt: Date.now() + data.expires_in * 1000,
		};

		return data.access_token;
	}
}
