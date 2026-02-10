/**
 * Database Migration Runner
 *
 * Runs DrizzleORM migrations using a single non-pooled connection.
 */

import { migrate } from 'drizzle-orm/postgres-js/migrator';
import { createMigrationDatabase } from './connection.js';

/**
 * Run database migrations from the specified folder.
 *
 * Uses a single non-pooled connection that is closed after migrations complete.
 *
 * @param databaseUrl - PostgreSQL connection URL
 * @param migrationsFolder - Path to the folder containing migration files
 */
export async function runMigrations(databaseUrl: string, migrationsFolder: string): Promise<void> {
  const database = createMigrationDatabase({ url: databaseUrl });
  try {
    await migrate(database.db, { migrationsFolder });
  } finally {
    await database.close();
  }
}
