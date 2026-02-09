/**
 * Pack drizzle migrations into a single JSON file for SEA embedding.
 *
 * Reads the drizzle/ directory and produces dist/migrations.json containing:
 * {
 *   "journal": "<contents of meta/_journal.json>",
 *   "files": { "0000_xxx.sql": "CREATE TABLE...", ... }
 * }
 *
 * Usage: node scripts/pack-migrations.js
 *
 * Prerequisites:
 * - Run `drizzle-kit generate` first to populate the drizzle/ directory
 */

import { readFileSync, writeFileSync, readdirSync, existsSync, mkdirSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const drizzleDir = resolve(__dirname, '../drizzle');
const distDir = resolve(__dirname, '../dist');
const outputPath = resolve(distDir, 'migrations.json');

if (!existsSync(drizzleDir)) {
	console.error(`Drizzle migrations directory not found: ${drizzleDir}`);
	console.error('Run `pnpm db:generate` first to generate migrations.');
	process.exit(1);
}

const journalPath = resolve(drizzleDir, 'meta', '_journal.json');
if (!existsSync(journalPath)) {
	console.error(`Journal file not found: ${journalPath}`);
	console.error('Run `pnpm db:generate` first to generate migrations.');
	process.exit(1);
}

const journal = readFileSync(journalPath, 'utf8');

const files = {};
for (const entry of readdirSync(drizzleDir)) {
	if (entry.endsWith('.sql')) {
		files[entry] = readFileSync(resolve(drizzleDir, entry), 'utf8');
	}
}

const sqlCount = Object.keys(files).length;
if (sqlCount === 0) {
	console.error('No .sql migration files found in drizzle/ directory.');
	process.exit(1);
}

mkdirSync(distDir, { recursive: true });
writeFileSync(outputPath, JSON.stringify({ journal, files }, null, 2));

console.log(`Packed ${sqlCount} migration(s) into ${outputPath}`);
