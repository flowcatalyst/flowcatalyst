/**
 * Secret Provider Interface
 *
 * Abstraction for fetching database credentials from different secret stores.
 * Implementations live in the app layer (cloud SDKs are opt-in); this package
 * only defines the contract.
 *
 * Built-in providers (registered by the app):
 *   - 'env'  — reads DATABASE_URL from the environment (default, no rotation)
 *   - 'aws'  — AWS Secrets Manager (opt-in, app must register)
 *   - 'gcp'  — GCP Secret Manager (opt-in, app must register)
 */

/**
 * A provider that returns the current database connection URL on demand.
 * Called once at startup and again on each polling interval.
 */
export interface SecretProvider {
	/** Human-readable name used in log messages. */
	readonly name: string;

	/** Return the current database URL. May perform a network call. */
	getDbUrl(): Promise<string>;
}

/**
 * Parse a secret value that may be either a plain connection URL or an AWS
 * RDS-style JSON object ({ username, password, host, port, dbname }).
 */
export function parseSecretToDbUrl(raw: string): string {
	const trimmed = raw.trim();
	if (trimmed.startsWith("{")) {
		const obj = JSON.parse(trimmed) as {
			username?: string;
			password?: string;
			host?: string;
			port?: number | string;
			dbname?: string;
			// Some rotation lambdas use "db" instead of "dbname"
			db?: string;
		};
		const { username, password, host, port = 5432, dbname, db } = obj;
		const database = dbname ?? db;
		if (!username || !password || !host || !database) {
			throw new Error(
				"Secret JSON is missing required fields: username, password, host, dbname",
			);
		}
		return `postgres://${encodeURIComponent(username)}:${encodeURIComponent(password)}@${host}:${port}/${database}?ssl=true`;
	}
	// Plain URL — return as-is
	return trimmed;
}
