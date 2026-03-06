/**
 * Database Connection
 *
 * Factory for creating DrizzleORM database instances with PostgreSQL.
 * Supports connection pooling and configuration options.
 */

import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";

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
		// Keep TCP connections alive so AWS NAT gateways / NLBs don't silently
		// drop idle sockets (default idle timeout is ~350s).
		keep_alive: 60,
	};

	if (config.debug) {
		options.debug = (_connection, query, params) => {
			console.log("[SQL]", query, params);
		};
	}

	const client = postgres(config.url, options);

	const db = drizzle({ client });

	return {
		db,
		client,
		async close() {
			await client.end();
		},
	};
}

/**
 * A Database whose underlying connection pool can be swapped out live without
 * restarting the process. Consumers hold references to `db` and `client` —
 * both are transparent JS Proxies that delegate every access to the current
 * real instances. Call `refresh(newUrl)` to swap in a new pool.
 */
export interface RefreshableDatabase extends Database {
	/**
	 * Replace the connection pool with one using the new URL.
	 * In-flight queries on the old pool are drained gracefully before it closes.
	 */
	refresh(newUrl: string): Promise<void>;
}

/**
 * Create a refreshable database whose `db` and `client` properties are proxy
 * objects that always delegate to the current underlying connection. When
 * `refresh(newUrl)` is called, a new pool is created and the proxies start
 * routing to it; the old pool is then drained and closed.
 */
export function createRefreshableDatabase(
	config: DatabaseConfig,
): RefreshableDatabase {
	const poolOptions: postgres.Options<Record<string, never>> = {
		max: config.maxConnections ?? 10,
		idle_timeout: config.idleTimeout ?? 20,
		connect_timeout: config.connectTimeout ?? 30,
		keep_alive: 60,
	};

	if (config.debug) {
		poolOptions.debug = (_connection, query, params) => {
			console.log("[SQL]", query, params);
		};
	}

	// Mutable holder — swapped on refresh
	const holder: {
		client: postgres.Sql;
		db: ReturnType<typeof drizzle>;
	} = (() => {
		const client = postgres(config.url, poolOptions);
		return { client, db: drizzle({ client }) };
	})();

	// Proxy for the postgres.js client (callable tagged-template + methods)
	const clientProxy = new Proxy(function () {} as unknown as postgres.Sql, {
		apply(_target, _thisArg, args) {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			return (holder.client as any)(...args);
		},
		get(_target, prop) {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const val = (holder.client as any)[prop];
			return typeof val === "function" ? val.bind(holder.client) : val;
		},
	});

	// Proxy for the drizzle db instance
	const dbProxy = new Proxy(
		{} as ReturnType<typeof drizzle>,
		{
			get(_target, prop) {
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				const val = (holder.db as any)[prop];
				return typeof val === "function" ? val.bind(holder.db) : val;
			},
		},
	);

	async function refresh(newUrl: string): Promise<void> {
		const newClient = postgres(newUrl, poolOptions);
		const newDb = drizzle({ client: newClient });

		const oldClient = holder.client;

		// Swap so new queries immediately route to the new pool
		holder.client = newClient;
		holder.db = newDb;

		// Drain and close the old pool (won't interrupt in-flight queries)
		await oldClient.end();
	}

	return {
		db: dbProxy,
		client: clientProxy,
		async close() {
			await holder.client.end();
		},
		refresh,
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
	const db = drizzle({ client });

	return {
		db,
		client,
		async close() {
			await client.end();
		},
	};
}
