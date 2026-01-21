/**
 * Database Connection
 *
 * Factory for creating DrizzleORM database instances with PostgreSQL.
 * Supports connection pooling and configuration options.
 */

import { drizzle } from 'drizzle-orm/postgres-js';
import postgres from 'postgres';

/**
 * Database connection configuration.
 */
export interface DatabaseConfig {
	/** PostgreSQL connection URL (e.g., postgres://user:pass@host:5432/db) */
	readonly url: string;
	/** Maximum number of connections in pool (default: 10) */
	readonly maxConnections?: number;
	/** Idle timeout in seconds (default: 20) */
	readonly idleTimeout?: number;
	/** Connection timeout in seconds (default: 30) */
	readonly connectTimeout?: number;
	/** Whether to log queries (default: false) */
	readonly debug?: boolean;
}

/**
 * Database instance with connection pool.
 */
export interface Database {
	/** DrizzleORM database instance */
	readonly db: ReturnType<typeof drizzle>;
	/** Underlying postgres.js client for raw queries */
	readonly client: postgres.Sql;
	/** Close all connections */
	close(): Promise<void>;
}

/**
 * Create a database connection with the given configuration.
 *
 * @param config - Database configuration
 * @returns Database instance with connection pool
 *
 * @example
 * ```typescript
 * const database = createDatabase({
 *     url: process.env.DATABASE_URL,
 *     maxConnections: 20,
 *     debug: process.env.NODE_ENV !== 'production',
 * });
 *
 * // Use in your application
 * const users = await database.db.select().from(usersTable);
 *
 * // On shutdown
 * await database.close();
 * ```
 */
export function createDatabase(config: DatabaseConfig): Database {
	const options: postgres.Options<Record<string, never>> = {
		max: config.maxConnections ?? 10,
		idle_timeout: config.idleTimeout ?? 20,
		connect_timeout: config.connectTimeout ?? 30,
	};

	if (config.debug) {
		options.debug = (_connection, query, params) => {
			console.log('[SQL]', query, params);
		};
	}

	const client = postgres(config.url, options);

	const db = drizzle(client);

	return {
		db,
		client,
		async close() {
			await client.end();
		},
	};
}

/**
 * Create a database for migrations (single connection, not pooled).
 *
 * @param config - Database configuration
 * @returns Database instance for migrations
 */
export function createMigrationDatabase(config: DatabaseConfig): Database {
	const client = postgres(config.url, { max: 1 });
	const db = drizzle(client);

	return {
		db,
		client,
		async close() {
			await client.end();
		},
	};
}
