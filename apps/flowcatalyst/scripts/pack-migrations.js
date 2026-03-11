/**
 * Pack drizzle migrations into a single JSON file for SEA embedding.
 *
 * Reads the drizzle/ directory and produces dist/migrations.json containing:
 * {
 *   "files": { "<timestamp>_<name>/migration.sql": "CREATE TABLE...", ... }
 * }
 *
 * Supports drizzle-kit v1 subdirectory format (each migration is a folder
 * containing migration.sql). Compatible with drizzle-orm's readMigrationFiles().
 *
 * Usage: node scripts/pack-migrations.js
 */

import {
	readFileSync,
	writeFileSync,
	readdirSync,
	statSync,
	existsSync,
	mkdirSync,
} from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const drizzleDir = resolve(__dirname, "../drizzle");
const distDir = resolve(__dirname, "../dist");
const outputPath = resolve(distDir, "migrations.json");

if (!existsSync(drizzleDir)) {
	console.error(`Drizzle migrations directory not found: ${drizzleDir}`);
	process.exit(1);
}

// Scan for migration subdirectories (each contains migration.sql)
const migrationDirs = readdirSync(drizzleDir)
	.filter((entry) => {
		const fullPath = resolve(drizzleDir, entry);
		return (
			statSync(fullPath).isDirectory() &&
			existsSync(resolve(fullPath, "migration.sql"))
		);
	})
	.sort();

if (migrationDirs.length === 0) {
	console.error("No migration subdirectories found in drizzle/ directory.");
	console.error("Expected folders like: drizzle/<timestamp>_<name>/migration.sql");
	process.exit(1);
}

const files = {};
for (const dir of migrationDirs) {
	const sqlPath = resolve(drizzleDir, dir, "migration.sql");
	files[`${dir}/migration.sql`] = readFileSync(sqlPath, "utf8");
}

mkdirSync(distDir, { recursive: true });
writeFileSync(outputPath, JSON.stringify({ files }, null, 2));

console.log(`Packed ${migrationDirs.length} migration(s) into ${outputPath}`);
