/**
 * FlowCatalyst App
 *
 * Production bundle that runs Platform and Stream Processor in a single process.
 * This is the production equivalent of dev-build, without the embedded router
 * and dev defaults.
 *
 * The Message Router runs as a separate deployment in production.
 *
 * Services:
 * - Platform: IAM, OIDC, Admin API
 * - Stream Processor: CQRS read model projections
 */

import { config } from 'dotenv';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createLogger, setDefaultLogger } from '@flowcatalyst/logging';

// Load .env file if present
const __dirname = dirname(fileURLToPath(import.meta.url));
config({ path: resolve(__dirname, '../.env') });

// Configuration
const PLATFORM_PORT = Number(process.env['PORT'] ?? process.env['PLATFORM_PORT'] ?? '3000');
const HOST = process.env['HOST'] ?? '0.0.0.0';
const LOG_LEVEL = (process.env['LOG_LEVEL'] ?? 'info') as 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';
const DATABASE_URL = process.env['DATABASE_URL'];

if (!DATABASE_URL) {
	console.error('DATABASE_URL is required');
	process.exit(1);
}

// Initialize logger
const logger = createLogger({
	level: LOG_LEVEL,
	serviceName: 'flowcatalyst-app',
	pretty: process.env['NODE_ENV'] === 'development',
});
setDefaultLogger(logger);

// Track started services for shutdown
type StopFn = () => Promise<void>;
const stopFns: StopFn[] = [];

async function shutdown(signal: string) {
	logger.info({ signal }, 'Shutting down...');

	for (const stop of stopFns.reverse()) {
		try {
			await stop();
		} catch (err) {
			logger.error({ err }, 'Error during shutdown');
		}
	}

	process.exit(0);
}

async function main() {
	logger.info(
		{ port: PLATFORM_PORT, host: HOST },
		'Starting FlowCatalyst App',
	);

	// 1. Start Platform
	const { startPlatform } = await import('@flowcatalyst/platform');
	const platformInstance = await startPlatform({
		port: PLATFORM_PORT,
		host: HOST,
		databaseUrl: DATABASE_URL,
		logLevel: LOG_LEVEL,
	});
	stopFns.push(async () => {
		await platformInstance.close();
	});

	// 2. Start Stream Processor
	const { startStreamProcessor } = await import('@flowcatalyst/stream-processor');
	const streamHandle = await startStreamProcessor({
		databaseUrl: DATABASE_URL,
		logLevel: LOG_LEVEL,
	});
	stopFns.push(async () => {
		await streamHandle.stop();
	});

	logger.info({ port: PLATFORM_PORT }, 'FlowCatalyst App started');

	process.on('SIGINT', () => shutdown('SIGINT'));
	process.on('SIGTERM', () => shutdown('SIGTERM'));

	// Keep process alive
	await new Promise(() => {});
}

main().catch((err) => {
	logger.error({ err }, 'Failed to start app');
	process.exit(1);
});
