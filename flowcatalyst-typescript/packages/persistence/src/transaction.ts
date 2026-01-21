/**
 * Transaction Management
 *
 * Transaction context and utilities for atomic database operations.
 * Uses postgres.js transactions with DrizzleORM.
 */

import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type postgres from 'postgres';

/**
 * Transaction context passed to repository operations.
 * Contains the database instance scoped to the current transaction.
 */
export interface TransactionContext {
	/** DrizzleORM database instance scoped to this transaction */
	readonly db: PostgresJsDatabase;
}

/**
 * Transaction manager for executing atomic operations.
 */
export interface TransactionManager {
	/**
	 * Execute a function within a database transaction.
	 * If the function throws, the transaction is rolled back.
	 * If the function returns, the transaction is committed.
	 *
	 * @param fn - The function to execute within the transaction
	 * @returns The result of the function
	 * @throws Re-throws any error from the function after rolling back
	 */
	inTransaction<T>(fn: (tx: TransactionContext) => Promise<T>): Promise<T>;

	/**
	 * Get the database instance (for non-transactional queries).
	 */
	readonly db: PostgresJsDatabase;
}

/**
 * Create a transaction manager from a DrizzleORM database instance.
 *
 * @param db - DrizzleORM database instance
 * @returns Transaction manager
 */
export function createTransactionManager(db: PostgresJsDatabase): TransactionManager {
	return {
		db,
		async inTransaction<T>(fn: (tx: TransactionContext) => Promise<T>): Promise<T> {
			return db.transaction(async (tx) => {
				return fn({ db: tx as unknown as PostgresJsDatabase });
			});
		},
	};
}

/**
 * Resolve the database instance from a transaction context or fall back to default.
 *
 * @param defaultDb - The default database instance
 * @param tx - Optional transaction context
 * @returns The database instance to use
 */
export function resolveDb(defaultDb: PostgresJsDatabase, tx?: TransactionContext): PostgresJsDatabase {
	return tx?.db ?? defaultDb;
}
