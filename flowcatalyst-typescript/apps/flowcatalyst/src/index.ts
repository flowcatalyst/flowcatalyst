/**
 * FlowCatalyst
 *
 * Unified app that runs Platform, Message Router, and Stream Processor
 * in a single process. Feature flags control which services are enabled.
 *
 * Services:
 * - Platform: IAM, OIDC, Admin API
 * - Stream Processor: CQRS read model projections
 * - Message Router: Queue processing, routing
 */

import { config } from "dotenv";
import { resolve, dirname, join } from "node:path";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { createLogger, setDefaultLogger } from "@flowcatalyst/logging";
import type { PlatformResult } from "@flowcatalyst/platform";

const VERSION = "0.0.1";

// Load .env file from app directory
const __dirname = dirname(fileURLToPath(import.meta.url));
config({ path: resolve(__dirname, "../.env") });

function printUsage() {
	console.log(`flowcatalyst v${VERSION}

Usage: flowcatalyst [command]

Commands:
  serve        Start all enabled services (default)
  migrate      Run database migrations and exit
  rotate-keys  Generate a new JWT signing key pair (for zero-downtime rotation)
  version      Print version and exit
  help         Show this help message

rotate-keys options:
  --key-dir <path>  Key directory (default: JWT_KEY_DIR or .jwt-keys)
  --keep <n>        Number of key pairs to retain (default: 2)

Environment:
  DATABASE_URL             PostgreSQL connection string (required)
  PLATFORM_ENABLED         Enable Platform service (default: true)
  STREAM_PROCESSOR_ENABLED Enable Stream Processor (default: true)
  MESSAGE_ROUTER_ENABLED   Enable Message Router (default: false)
  PORT / PLATFORM_PORT     Platform HTTP port (default: 3000)
  ROUTER_PORT              Message Router port (default: 8080)
  AUTO_MIGRATE             Auto-run migrations on serve (default: true in dev)
  JWT_KEY_DIR              Directory for JWT key pairs (rotation-capable)`);
}

// Resolve migrations folder — SEA asset, dist/drizzle, or ../drizzle
async function resolveMigrationsFolder(): Promise<string> {
	// SEA: extract embedded migrations to temp dir
	try {
		const sea = await import("node:sea");
		if (sea.isSea()) {
			const raw = sea.getAsset("migrations", "utf8");
			const data = JSON.parse(raw) as {
				journal: string;
				files: Record<string, string>;
			};
			const dir = join(tmpdir(), "flowcatalyst-migrations");
			mkdirSync(join(dir, "meta"), { recursive: true });
			writeFileSync(join(dir, "meta", "_journal.json"), data.journal);
			for (const [name, content] of Object.entries(data.files)) {
				writeFileSync(join(dir, name), content);
			}
			return dir;
		}
	} catch {
		// Not running as SEA, fall through to filesystem
	}

	const distDrizzle = resolve(__dirname, "drizzle");
	if (existsSync(distDrizzle)) return distDrizzle;
	return resolve(__dirname, "../drizzle");
}

// Resolve frontend dir — SEA asset, dist/frontend, or sibling platform-frontend/dist
async function resolveFrontendDir(): Promise<string | undefined> {
	// SEA: extract embedded frontend to temp dir
	try {
		const sea = await import("node:sea");
		if (sea.isSea()) {
			const raw = sea.getAsset("frontend", "utf8");
			const data = JSON.parse(raw) as {
				files: Record<string, { content: string; encoding: "utf8" | "base64" }>;
			};
			const dir = join(tmpdir(), "flowcatalyst-frontend");
			for (const [relPath, file] of Object.entries(data.files)) {
				const fullPath = join(dir, relPath);
				mkdirSync(dirname(fullPath), { recursive: true });
				writeFileSync(fullPath, Buffer.from(file.content, file.encoding));
			}
			return dir;
		}
	} catch {
		// Not running as SEA
	}

	// Filesystem: check dist/frontend then sibling platform-frontend/dist
	const distFrontend = resolve(__dirname, "frontend");
	if (existsSync(distFrontend)) return distFrontend;
	const siblingFrontend = resolve(__dirname, "../../platform-frontend/dist");
	if (existsSync(siblingFrontend)) return siblingFrontend;
	return undefined;
}

function parseArgs(args: string[]): Record<string, string> {
	const result: Record<string, string> = {};
	for (let i = 0; i < args.length; i++) {
		const arg = args[i]!;
		if (arg.startsWith("--") && i + 1 < args.length) {
			result[arg.slice(2)] = args[i + 1]!;
			i++;
		}
	}
	return result;
}

async function runRotateKeysCommand(): Promise<void> {
	const args = parseArgs(process.argv.slice(3));
	const keyDir = args["key-dir"] ?? process.env["JWT_KEY_DIR"] ?? ".jwt-keys";
	const keep = Number(args["keep"] ?? "2");

	if (keep < 1) {
		console.error("--keep must be at least 1");
		process.exit(1);
	}

	const { generateKeyPair, writeKeyPair, loadKeyDir, removeKeyPair } =
		await import("@flowcatalyst/platform");

	// Generate new key pair
	const { kid, privatePem, publicPem } = await generateKeyPair();
	await writeKeyPair(keyDir, kid, privatePem, publicPem);
	console.log(`Generated new key pair: ${kid}`);
	console.log(`  ${keyDir}/${kid}.private.pem`);
	console.log(`  ${keyDir}/${kid}.public.pem`);

	// Load all pairs and prune if over --keep
	const pairs = await loadKeyDir(keyDir);
	if (pairs.length > keep) {
		const toRemove = pairs.slice(0, pairs.length - keep);
		for (const pair of toRemove) {
			await removeKeyPair(keyDir, pair.kid);
			console.log(`Pruned old key: ${pair.kid}`);
		}
	}

	console.log(`\nActive keys (${Math.min(pairs.length, keep)}):`);
	const remaining = await loadKeyDir(keyDir);
	for (const pair of remaining) {
		const label = pair.kid === kid ? " (signing)" : " (validation only)";
		console.log(`  ${pair.kid}${label}`);
	}

	console.log("\nRestart the service to use the new signing key.");
}

async function runMigrateCommand(): Promise<void> {
	const url = process.env["DATABASE_URL"];
	if (!url) {
		console.error("DATABASE_URL is required");
		process.exit(1);
	}
	const migrationsFolder = await resolveMigrationsFolder();
	const { runMigrations } = await import("@flowcatalyst/persistence");
	console.log("Running database migrations...");
	await runMigrations(url, migrationsFolder);
	console.log("Migrations complete.");
}

// --- CLI Command Routing ---
const command = process.argv[2] ?? "serve";

switch (command) {
	case "serve":
		break; // fall through to main()
	case "migrate":
		await runMigrateCommand();
		process.exit(0);
	case "rotate-keys":
		await runRotateKeysCommand();
		process.exit(0);
	case "version":
	case "--version":
	case "-v":
		console.log(`flowcatalyst v${VERSION}`);
		process.exit(0);
	case "help":
	case "--help":
	case "-h":
		printUsage();
		process.exit(0);
	default:
		console.error(`Unknown command: ${command}\n`);
		printUsage();
		process.exit(1);
}

// --- Serve command: load config and start services ---

// Configuration
const NODE_ENV = process.env["NODE_ENV"] ?? "development";
const isDev = NODE_ENV === "development";
const LOG_LEVEL = (process.env["LOG_LEVEL"] ?? "info") as
	| "trace"
	| "debug"
	| "info"
	| "warn"
	| "error"
	| "fatal";
const PLATFORM_PORT = Number(
	process.env["PORT"] ?? process.env["PLATFORM_PORT"] ?? "3000",
);
const ROUTER_PORT = Number(process.env["ROUTER_PORT"] ?? "8080");
const HOST = process.env["HOST"] ?? "0.0.0.0";
const DATABASE_URL = (() => {
	const url = process.env["DATABASE_URL"];
	if (!url) {
		console.error("DATABASE_URL is required");
		process.exit(1);
	}
	return url;
})();

// Frontend dir override
const FRONTEND_DIR = process.env["FRONTEND_DIR"];

// Feature flags
const PLATFORM_ENABLED = process.env["PLATFORM_ENABLED"] !== "false";
const MESSAGE_ROUTER_ENABLED = process.env["MESSAGE_ROUTER_ENABLED"] === "true";
const STREAM_PROCESSOR_ENABLED =
	process.env["STREAM_PROCESSOR_ENABLED"] !== "false";
const OUTBOX_PROCESSOR_ENABLED =
	process.env["OUTBOX_PROCESSOR_ENABLED"] === "true";
const AUTO_MIGRATE =
	process.env["AUTO_MIGRATE"] !== undefined
		? process.env["AUTO_MIGRATE"] === "true"
		: isDev;

// Set env defaults for message router when enabled
if (MESSAGE_ROUTER_ENABLED) {
	process.env["QUEUE_TYPE"] = process.env["QUEUE_TYPE"] ?? "EMBEDDED";
	process.env["EMBEDDED_DB_PATH"] =
		process.env["EMBEDDED_DB_PATH"] ?? ":memory:";
	process.env["OIDC_ISSUER_URL"] =
		process.env["OIDC_ISSUER_URL"] ?? `http://localhost:${PLATFORM_PORT}`;
	process.env["PLATFORM_URL"] =
		process.env["PLATFORM_URL"] ?? `http://localhost:${PLATFORM_PORT}`;
}
process.env["DATABASE_URL"] = DATABASE_URL;

// Initialize logger
const logger = createLogger({
	level: LOG_LEVEL,
	serviceName: "flowcatalyst",
	pretty: isDev,
});
setDefaultLogger(logger);

// Track started services for shutdown
type StopFn = () => Promise<void>;
const stopFns: StopFn[] = [];

async function shutdown(signal: string) {
	logger.info({ signal }, "Shutting down...");

	for (const stop of stopFns.reverse()) {
		try {
			await stop();
		} catch (err) {
			logger.error({ err }, "Error during shutdown");
		}
	}

	process.exit(0);
}

async function main() {
	const enabledServices = [
		PLATFORM_ENABLED && "Platform",
		STREAM_PROCESSOR_ENABLED && "Stream Processor",
		MESSAGE_ROUTER_ENABLED && "Message Router",
		OUTBOX_PROCESSOR_ENABLED && "Outbox Processor",
	].filter(Boolean);

	logger.info(
		{
			services: enabledServices,
			platformPort: PLATFORM_ENABLED ? PLATFORM_PORT : undefined,
			routerPort: MESSAGE_ROUTER_ENABLED ? ROUTER_PORT : undefined,
			env: NODE_ENV,
			autoMigrate: AUTO_MIGRATE,
		},
		"Starting FlowCatalyst",
	);

	// Startup banner
	const lines = [
		`  Services:`,
		PLATFORM_ENABLED &&
			`    Platform (IAM/OIDC):    http://localhost:${PLATFORM_PORT}`,
		STREAM_PROCESSOR_ENABLED && `    Stream Processor:       running (same DB)`,
		MESSAGE_ROUTER_ENABLED &&
			`    Message Router:         http://localhost:${ROUTER_PORT}`,
		OUTBOX_PROCESSOR_ENABLED &&
			`    Outbox Processor:       running (external DB)`,
	].filter(Boolean);

	console.log(`\n${lines.join("\n")}\n`);

	// Run migrations if enabled
	if (AUTO_MIGRATE) {
		logger.info("Running database migrations...");
		const { runMigrations } = await import("@flowcatalyst/persistence");
		const migrationsFolder = await resolveMigrationsFolder();
		await runMigrations(DATABASE_URL, migrationsFolder);
		logger.info("Migrations complete");
	}

	// 1. Start Platform
	let platformResult: PlatformResult | null = null;

	if (PLATFORM_ENABLED) {
		logger.info({ port: PLATFORM_PORT }, "Starting Platform...");
		const { startPlatform } = await import("@flowcatalyst/platform");
		const frontendDir = FRONTEND_DIR ?? (await resolveFrontendDir());
		if (frontendDir) {
			logger.info({ frontendDir }, "Frontend assets detected");
		}
		platformResult = await startPlatform({
			port: PLATFORM_PORT,
			host: HOST,
			databaseUrl: DATABASE_URL,
			logLevel: LOG_LEVEL,
			frontendDir,
		});
		stopFns.push(async () => {
			logger.info("Stopping Platform...");
			await platformResult!.server.close();
		});
	}

	// 2. Start Stream Processor
	if (STREAM_PROCESSOR_ENABLED) {
		logger.info("Starting Stream Processor...");
		const { startStreamProcessor } = await import(
			"@flowcatalyst/stream-processor"
		);
		const streamHandle = await startStreamProcessor({
			databaseUrl: DATABASE_URL,
			logLevel: LOG_LEVEL,
		});
		stopFns.push(async () => {
			logger.info("Stopping Stream Processor...");
			await streamHandle.stop();
		});
	}

	// 3. Start Message Router
	if (MESSAGE_ROUTER_ENABLED) {
		logger.info({ port: ROUTER_PORT }, "Starting Message Router...");
		const { startRouter } = await import("@flowcatalyst/message-router");
		const { server: routerServer, services: routerServices } =
			await startRouter({
				port: ROUTER_PORT,
				host: HOST,
				logLevel: LOG_LEVEL,
			});
		stopFns.push(async () => {
			logger.info("Stopping Message Router...");
			routerServer.close();
			routerServices.brokerHealth.stop();
			routerServices.queueHealthMonitor.stop();
			await routerServices.notifications.stop();
			await routerServices.queueManager.stop();
		});

		// Wire embedded post-commit dispatch: Platform → embedded queue → Message Router
		if (platformResult && routerServices.queueManager.hasEmbeddedQueue()) {
			const { createEmbeddedPublisher } = await import(
				"./queue-core/publisher/embedded-publisher.js"
			);
			const { createPostCommitDispatcherFromPublisher } = await import(
				"@flowcatalyst/platform"
			);
			const publisher = createEmbeddedPublisher((msg) => {
				const queueMsg: Parameters<
					typeof routerServices.queueManager.publishToEmbeddedQueue
				>[0] = {
					messageId: msg.messageId,
					messageGroupId: msg.messageGroupId,
					payload: msg.payload,
				};
				if (msg.messageDeduplicationId !== undefined) {
					queueMsg.messageDeduplicationId = msg.messageDeduplicationId;
				}
				return routerServices.queueManager.publishToEmbeddedQueue(queueMsg);
			});
			platformResult.setPostCommitDispatcher(
				createPostCommitDispatcherFromPublisher(publisher),
			);
			logger.info(
				"Embedded post-commit dispatch wired (Platform → Message Router)",
			);
		}
	}

	// 4. Start Outbox Processor
	if (OUTBOX_PROCESSOR_ENABLED) {
		logger.info("Starting Outbox Processor...");
		const { startOutboxProcessor } = await import(
			"./outbox-processor/index.js"
		);
		const outboxHandle = await startOutboxProcessor();
		stopFns.push(async () => {
			logger.info("Stopping Outbox Processor...");
			await outboxHandle.stop();
		});
	}

	if (enabledServices.length === 0) {
		logger.warn(
			"No services enabled. Set PLATFORM_ENABLED, STREAM_PROCESSOR_ENABLED, or MESSAGE_ROUTER_ENABLED to true.",
		);
		process.exit(1);
	}

	logger.info("All services started successfully");

	process.on("SIGINT", () => shutdown("SIGINT"));
	process.on("SIGTERM", () => shutdown("SIGTERM"));

	// Keep process alive
	await new Promise(() => {});
}

main().catch((err) => {
	logger.error({ err }, "Failed to start FlowCatalyst");
	process.exit(1);
});
