/**
 * Configuration
 *
 * Reads FlowCatalyst connection settings from environment variables.
 */

export interface Config {
	/** Base URL of the FlowCatalyst platform */
	readonly baseUrl: string;
	/** OAuth2 client ID (service account) */
	readonly clientId: string;
	/** OAuth2 client secret */
	readonly clientSecret: string;
}

export function loadConfig(): Config {
	const baseUrl = requireEnv("FLOWCATALYST_URL");
	const clientId = requireEnv("FLOWCATALYST_CLIENT_ID");
	const clientSecret = requireEnv("FLOWCATALYST_CLIENT_SECRET");

	return {
		baseUrl: baseUrl.replace(/\/$/, ""),
		clientId,
		clientSecret,
	};
}

function requireEnv(name: string): string {
	const value = process.env[name];
	if (!value) {
		throw new Error(`Missing required environment variable: ${name}`);
	}
	return value;
}
