/**
 * FlowCatalyst Dev Build
 *
 * Combined development server that runs Platform, Message Router, and
 * Stream Processor in a single Node.js process.
 *
 * This mirrors the Java flowcatalyst-dev-build module which combines
 * all FlowCatalyst components for local development.
 *
 * Services:
 * - Platform: IAM, OIDC, Admin API (port 3000)
 * - Message Router: Queue processing, routing (port 8080)
 * - Stream Processor: CQRS read model projections (same DB)
 */

import { config } from 'dotenv';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createLogger, setDefaultLogger } from '@flowcatalyst/logging';

// Load .env file from dev-build directory
const __dirname = dirname(fileURLToPath(import.meta.url));
config({ path: resolve(__dirname, '../.env') });

// Configuration
const PLATFORM_PORT = Number(process.env['PLATFORM_PORT'] ?? '3000');
const ROUTER_PORT = Number(process.env['ROUTER_PORT'] ?? '8080');
const LOG_LEVEL = (process.env['LOG_LEVEL'] ?? 'info') as 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';
const NODE_ENV = process.env['NODE_ENV'] ?? 'development';
const DATABASE_URL = process.env['DATABASE_URL'] ?? 'postgres://localhost:5432/flowcatalyst';

// Set dev environment defaults before importing services
process.env['NODE_ENV'] = NODE_ENV;
process.env['QUEUE_TYPE'] = process.env['QUEUE_TYPE'] ?? 'EMBEDDED';
process.env['EMBEDDED_DB_PATH'] = process.env['EMBEDDED_DB_PATH'] ?? ':memory:';
process.env['OIDC_ISSUER_URL'] = process.env['OIDC_ISSUER_URL'] ?? `http://localhost:${PLATFORM_PORT}`;
process.env['PLATFORM_URL'] = process.env['PLATFORM_URL'] ?? `http://localhost:${PLATFORM_PORT}`;
process.env['DATABASE_URL'] = DATABASE_URL;

// Initialize logger
const logger = createLogger({
	level: LOG_LEVEL,
	serviceName: 'dev-build',
	pretty: NODE_ENV === 'development',
});
setDefaultLogger(logger);

// Track started services for shutdown
type StopFn = () => Promise<void>;
const stopFns: StopFn[] = [];

/**
 * Graceful shutdown handler
 */
async function shutdown(signal: string) {
	logger.info({ signal }, 'Shutting down dev-build...');

	for (const stop of stopFns.reverse()) {
		try {
			await stop();
		} catch (err) {
			logger.error({ err }, 'Error during shutdown');
		}
	}

	process.exit(0);
}

// Main startup
async function main() {
	logger.info(
		{
			platformPort: PLATFORM_PORT,
			routerPort: ROUTER_PORT,
			env: NODE_ENV,
			queueType: process.env['QUEUE_TYPE'],
		},
		'Starting FlowCatalyst Dev Build',
	);

	console.log(`
╔═══════════════════════════════════════════════════════════════════╗
║                    FlowCatalyst Dev Build                       ║
╠═══════════════════════════════════════════════════════════════════╣
║  Platform (IAM/OIDC):    http://localhost:${String(PLATFORM_PORT).padEnd(5)}                 ║
║  Message Router:         http://localhost:${String(ROUTER_PORT).padEnd(5)}                 ║
║  Queue Type:             EMBEDDED (in-memory SQLite)            ║
╠═══════════════════════════════════════════════════════════════════╣
║  Endpoints:                                                     ║
║  ├─ Platform:                                                   ║
║  │  ├─ Health:     http://localhost:${PLATFORM_PORT}/health                     ║
║  │  ├─ OpenAPI:    http://localhost:${PLATFORM_PORT}/docs                       ║
║  │  ├─ OIDC:       http://localhost:${PLATFORM_PORT}/.well-known/openid-configuration
║  │  ├─ Admin API:  http://localhost:${PLATFORM_PORT}/api/admin/*                ║
║  │  └─ OAuth:      http://localhost:${PLATFORM_PORT}/oauth/*                    ║
║  ├─ Stream Processor:  running (same DB)                        ║
║  └─ Router:                                                     ║
║     ├─ Health:     http://localhost:${ROUTER_PORT}/health                      ║
║     ├─ Metrics:    http://localhost:${ROUTER_PORT}/metrics                     ║
║     └─ OpenAPI:    http://localhost:${ROUTER_PORT}/openapi.json                ║
╚═══════════════════════════════════════════════════════════════════╝
`);

	// 1. Start Platform
	logger.info({ port: PLATFORM_PORT }, 'Starting Platform service...');
	const { startPlatform } = await import('@flowcatalyst/platform');
	const platformInstance = await startPlatform({
		port: PLATFORM_PORT,
		host: '0.0.0.0',
		databaseUrl: DATABASE_URL,
		logLevel: LOG_LEVEL,
	});
	stopFns.push(async () => {
		logger.info('Stopping Platform...');
		await platformInstance.close();
	});

	// 2. Start Stream Processor
	logger.info('Starting Stream Processor...');
	const { startStreamProcessor } = await import('@flowcatalyst/stream-processor');
	const streamHandle = await startStreamProcessor({
		databaseUrl: DATABASE_URL,
		logLevel: LOG_LEVEL,
	});
	stopFns.push(async () => {
		logger.info('Stopping Stream Processor...');
		await streamHandle.stop();
	});

	// 3. Start Message Router
	logger.info({ port: ROUTER_PORT }, 'Starting Message Router...');
	const { startRouter } = await import('@flowcatalyst/message-router');
	const { server: routerServer, services: routerServices } = await startRouter({
		port: ROUTER_PORT,
		host: '0.0.0.0',
		logLevel: LOG_LEVEL,
	});
	stopFns.push(async () => {
		logger.info('Stopping Message Router...');
		routerServer.close();
		routerServices.brokerHealth.stop();
		routerServices.queueHealthMonitor.stop();
		await routerServices.notifications.stop();
		await routerServices.queueManager.stop();
	});

	logger.info('All services started successfully');

	// Handle shutdown signals
	process.on('SIGINT', () => shutdown('SIGINT'));
	process.on('SIGTERM', () => shutdown('SIGTERM'));

	// Keep process alive
	await new Promise(() => {});
}

main().catch((err) => {
	logger.error({ err }, 'Failed to start dev-build');
	process.exit(1);
});
