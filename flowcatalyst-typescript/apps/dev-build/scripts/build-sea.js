/**
 * Build a Node.js Single Executable Application (SEA).
 *
 * Steps:
 * 1. Copy the Node binary
 * 2. Remove existing signature (macOS)
 * 3. Inject the SEA blob using postject
 * 4. Re-sign the binary (macOS)
 *
 * Usage: node scripts/build-sea.js
 *
 * Prerequisites:
 * - Run `tsup` first to bundle dist/index.js
 * - Run `node --experimental-sea-config sea-config.json` to generate dist/sea-prep.blob
 */

import { execSync } from 'node:child_process';
import { copyFileSync, chmodSync, existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const distDir = resolve(__dirname, '../dist');
const blobPath = resolve(distDir, 'sea-prep.blob');
const outputName = process.platform === 'win32' ? 'flowcatalyst-dev.exe' : 'flowcatalyst-dev';
const outputPath = resolve(distDir, outputName);

function run(cmd, opts = {}) {
	console.log(`  $ ${cmd}`);
	execSync(cmd, { stdio: 'inherit', ...opts });
}

async function main() {
	console.log('Building FlowCatalyst Dev SEA...\n');

	// Verify blob exists
	if (!existsSync(blobPath)) {
		console.error(`SEA blob not found at ${blobPath}`);
		console.error('Run: node --experimental-sea-config sea-config.json');
		process.exit(1);
	}

	// 1. Copy Node binary
	console.log('1. Copying Node binary...');
	copyFileSync(process.execPath, outputPath);
	chmodSync(outputPath, 0o755);

	// 2. Remove signature on macOS
	if (process.platform === 'darwin') {
		console.log('2. Removing macOS code signature...');
		run(`codesign --remove-signature "${outputPath}"`);
	}

	// 3. Inject SEA blob using postject
	console.log('3. Injecting SEA blob...');
	const postjectArgs = [
		`"${outputPath}"`,
		'NODE_SEA_BLOB',
		`"${blobPath}"`,
		'--sentinel-fuse', 'NODE_SEA_FUSE_fce680ab2cc467b6e072b8b5df1996b2',
	];

	if (process.platform === 'darwin') {
		postjectArgs.push('--macho-segment-name', 'NODE_SEA');
	}

	run(`npx postject ${postjectArgs.join(' ')}`);

	// 4. Re-sign on macOS
	if (process.platform === 'darwin') {
		console.log('4. Re-signing binary for macOS...');
		run(`codesign --sign - "${outputPath}"`);
	}

	console.log(`\nSEA binary created: ${outputPath}`);
	console.log(`Run with: ${outputPath}`);
}

main().catch((err) => {
	console.error('SEA build failed:', err);
	process.exit(1);
});
