/**
 * FlowCatalyst Dev Build
 *
 * Combined development server that runs both Platform (IAM/OIDC) and
 * Message Router services in a single process.
 *
 * This mirrors the Java flowcatalyst-dev-build module which combines
 * all FlowCatalyst components for local development.
 *
 * Services:
 * - Platform: IAM, OIDC, Admin API (port 3000)
 * - Message Router: Queue processing, routing (port 8080)
 */

import { config } from 'dotenv';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, type ChildProcess } from 'node:child_process';
import { createLogger, setDefaultLogger } from '@flowcatalyst/logging';

// Load .env file from dev-build directory
const __dirname = dirname(fileURLToPath(import.meta.url));
config({ path: resolve(__dirname, '../.env') });

// Configuration
const PLATFORM_PORT = process.env['PLATFORM_PORT'] ?? '3000';
const ROUTER_PORT = process.env['ROUTER_PORT'] ?? '8080';
const LOG_LEVEL = process.env['LOG_LEVEL'] ?? 'info';
const NODE_ENV = process.env['NODE_ENV'] ?? 'development';

// Initialize logger
const logger = createLogger({
	level: LOG_LEVEL as 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal',
	serviceName: 'dev-build',
	pretty: NODE_ENV === 'development',
});
setDefaultLogger(logger);

// Track child processes for cleanup
const children: ChildProcess[] = [];

/**
 * Start the Platform service
 */
function startPlatform(): ChildProcess {
	logger.info({ port: PLATFORM_PORT }, 'Starting Platform service...');

	const platformDir = resolve(__dirname, '../../platform');
	const child = spawn('npx', ['tsx', 'src/index.ts'], {
		cwd: platformDir,
		stdio: ['inherit', 'pipe', 'pipe'],
		env: {
			...process.env,
			PORT: PLATFORM_PORT,
			NODE_ENV,
			LOG_LEVEL,
			// Platform-specific defaults for dev
			OIDC_DEV_INTERACTIONS: 'true',
		},
	});

	child.stdout?.on('data', (data) => {
		const lines = data.toString().trim().split('\n');
		for (const line of lines) {
			if (line) {
				try {
					const parsed = JSON.parse(line);
					logger.info({ ...parsed, service: 'platform' }, parsed.msg || 'Platform');
				} catch {
					logger.info({ service: 'platform' }, line);
				}
			}
		}
	});

	child.stderr?.on('data', (data) => {
		logger.error({ service: 'platform' }, data.toString().trim());
	});

	child.on('exit', (code) => {
		logger.info({ code, service: 'platform' }, 'Platform service exited');
	});

	return child;
}

/**
 * Start the Message Router service
 */
function startMessageRouter(): ChildProcess {
	logger.info({ port: ROUTER_PORT }, 'Starting Message Router service...');

	const routerDir = resolve(__dirname, '../../message-router');
	const child = spawn('npx', ['tsx', 'src/index.ts'], {
		cwd: routerDir,
		stdio: ['inherit', 'pipe', 'pipe'],
		env: {
			...process.env,
			PORT: ROUTER_PORT,
			NODE_ENV,
			LOG_LEVEL,
			// Use embedded broker for development
			QUEUE_TYPE: 'EMBEDDED',
			EMBEDDED_DB_PATH: ':memory:',
			// Point to local platform for OIDC
			OIDC_ISSUER_URL: `http://localhost:${PLATFORM_PORT}`,
			PLATFORM_URL: `http://localhost:${PLATFORM_PORT}`,
		},
	});

	child.stdout?.on('data', (data) => {
		const lines = data.toString().trim().split('\n');
		for (const line of lines) {
			if (line) {
				try {
					const parsed = JSON.parse(line);
					logger.info({ ...parsed, service: 'router' }, parsed.msg || 'Router');
				} catch {
					logger.info({ service: 'router' }, line);
				}
			}
		}
	});

	child.stderr?.on('data', (data) => {
		logger.error({ service: 'router' }, data.toString().trim());
	});

	child.on('exit', (code) => {
		logger.info({ code, service: 'router' }, 'Message Router service exited');
	});

	return child;
}

/**
 * Graceful shutdown handler
 */
function shutdown(signal: string) {
	logger.info({ signal }, 'Shutting down dev-build...');

	for (const child of children) {
		if (!child.killed) {
			child.kill('SIGTERM');
		}
	}

	// Give processes time to shut down gracefully
	setTimeout(() => {
		for (const child of children) {
			if (!child.killed) {
				child.kill('SIGKILL');
			}
		}
		process.exit(0);
	}, 5000);
}

// Main startup
async function main() {
	logger.info(
		{
			platformPort: PLATFORM_PORT,
			routerPort: ROUTER_PORT,
			env: NODE_ENV,
			queueType: 'EMBEDDED',
		},
		'Starting FlowCatalyst Dev Build',
	);

	console.log(`
╔═══════════════════════════════════════════════════════════════════╗
║                    FlowCatalyst Dev Build                         ║
╠═══════════════════════════════════════════════════════════════════╣
║  Platform (IAM/OIDC):    http://localhost:${PLATFORM_PORT.padEnd(5)}                   ║
║  Message Router:         http://localhost:${ROUTER_PORT.padEnd(5)}                   ║
║  Queue Type:             EMBEDDED (in-memory SQLite)              ║
╠═══════════════════════════════════════════════════════════════════╣
║  Endpoints:                                                       ║
║  ├─ Platform:                                                     ║
║  │  ├─ Health:     http://localhost:${PLATFORM_PORT}/health                       ║
║  │  ├─ OIDC:       http://localhost:${PLATFORM_PORT}/.well-known/openid-configuration
║  │  ├─ Admin API:  http://localhost:${PLATFORM_PORT}/api/admin/*                  ║
║  │  └─ OAuth:      http://localhost:${PLATFORM_PORT}/oauth/*                      ║
║  └─ Router:                                                       ║
║     ├─ Health:     http://localhost:${ROUTER_PORT}/health                        ║
║     ├─ Metrics:    http://localhost:${ROUTER_PORT}/metrics                       ║
║     ├─ Config:     http://localhost:${ROUTER_PORT}/api/config                    ║
║     └─ OpenAPI:    http://localhost:${ROUTER_PORT}/openapi.json                  ║
╚═══════════════════════════════════════════════════════════════════╝
`);

	// Start services
	const platform = startPlatform();
	children.push(platform);

	// Wait a bit for platform to start before starting router
	// (router may need to connect to platform for OIDC discovery)
	await new Promise((resolve) => setTimeout(resolve, 2000));

	const router = startMessageRouter();
	children.push(router);

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
