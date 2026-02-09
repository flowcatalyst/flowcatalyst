/**
 * Pack frontend dist into a single JSON file for SEA embedding.
 *
 * Recursively walks the platform-frontend dist/ directory and produces
 * dist/frontend.json containing:
 * {
 *   "files": {
 *     "index.html": { "content": "...", "encoding": "utf8" },
 *     "assets/main.js": { "content": "...", "encoding": "utf8" },
 *     "assets/logo.png": { "content": "base64...", "encoding": "base64" }
 *   }
 * }
 *
 * Usage: node scripts/pack-frontend.js
 *
 * Prerequisites:
 * - Run `pnpm --filter @flowcatalyst/platform-frontend build` first
 */

import { readFileSync, writeFileSync, readdirSync, statSync, existsSync, mkdirSync } from 'node:fs';
import { resolve, relative, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const frontendDir = resolve(__dirname, '../../platform-frontend/dist');
const distDir = resolve(__dirname, '../dist');
const outputPath = resolve(distDir, 'frontend.json');

const TEXT_EXTENSIONS = new Set(['.html', '.js', '.css', '.svg', '.json', '.txt', '.map', '.xml', '.webmanifest']);

if (!existsSync(frontendDir)) {
	console.error(`Frontend dist directory not found: ${frontendDir}`);
	console.error('Run `pnpm --filter @flowcatalyst/platform-frontend build` first.');
	process.exit(1);
}

function walkDir(dir) {
	const files = {};
	for (const entry of readdirSync(dir)) {
		const fullPath = resolve(dir, entry);
		const stat = statSync(fullPath);
		if (stat.isDirectory()) {
			Object.assign(files, walkDir(fullPath));
		} else {
			const relPath = relative(frontendDir, fullPath);
			const ext = entry.substring(entry.lastIndexOf('.'));
			if (TEXT_EXTENSIONS.has(ext)) {
				files[relPath] = { content: readFileSync(fullPath, 'utf8'), encoding: 'utf8' };
			} else {
				files[relPath] = { content: readFileSync(fullPath).toString('base64'), encoding: 'base64' };
			}
		}
	}
	return files;
}

const files = walkDir(frontendDir);
const fileCount = Object.keys(files).length;

if (fileCount === 0) {
	console.error('No files found in frontend dist/ directory.');
	process.exit(1);
}

mkdirSync(distDir, { recursive: true });
writeFileSync(outputPath, JSON.stringify({ files }, null, 2));

console.log(`Packed ${fileCount} frontend file(s) into ${outputPath}`);
