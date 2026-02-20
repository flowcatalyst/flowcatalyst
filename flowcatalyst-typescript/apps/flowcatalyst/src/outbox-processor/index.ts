/**
 * Outbox Processor
 *
 * Polls a customer's outbox_messages table and POSTs events, dispatch jobs,
 * and audit logs in batches to the platform's batch API endpoints.
 *
 * Architecture:
 *   Customer DB (outbox_messages)
 *     → OutboxPoller (interval-based)
 *     → GlobalBuffer (bounded queue, backpressure)
 *     → GroupDistributor (routes by type:messageGroup)
 *     → MessageGroupProcessor (FIFO per group, batches of 100)
 *     → ApiClient (POST /api/events/batch, etc.)
 *     → Update outbox_messages status
 */

import { createLogger } from "@flowcatalyst/logging";
import {
	loadOutboxProcessorConfig,
	type OutboxProcessorConfig,
} from "./env.js";
import { createPostgresOutboxRepository } from "./repository/postgres-repository.js";
import { createApiClient } from "./api-client.js";
import { createGlobalBuffer } from "./global-buffer.js";
import { createGroupDistributor } from "./group-distributor.js";
import { createOutboxPoller } from "./outbox-poller.js";

/**
 * Handle for controlling the outbox processor lifecycle.
 */
export interface OutboxProcessorHandle {
	stop(): Promise<void>;
}

/**
 * Start the outbox processor.
 *
 * @param configOverride - Optional config override (uses env vars if not provided)
 * @returns Handle for stopping the processor
 */
export async function startOutboxProcessor(
	configOverride?: Partial<OutboxProcessorConfig>,
): Promise<OutboxProcessorHandle> {
	const envConfig = loadOutboxProcessorConfig();
	const config: OutboxProcessorConfig = { ...envConfig, ...configOverride };

	const logger = createLogger({
		level: "info",
		serviceName: "outbox-processor",
	});

	logger.info(
		{
			apiBaseUrl: config.apiBaseUrl,
			pollIntervalMs: config.pollIntervalMs,
			pollBatchSize: config.pollBatchSize,
			apiBatchSize: config.apiBatchSize,
			maxConcurrentGroups: config.maxConcurrentGroups,
			globalBufferSize: config.globalBufferSize,
			maxInFlight: config.maxInFlight,
		},
		"Starting outbox processor",
	);

	// Create repository (customer DB connection)
	const repository = createPostgresOutboxRepository(config);

	// Create API client (platform batch endpoints)
	const apiClient = createApiClient(config, logger);

	// Create poller (we need a reference before creating the distributor)
	// Use a wrapper to break circular dependency
	let releaseInFlightFn: (count: number) => void = () => {};

	// Create group distributor
	const distributor = createGroupDistributor(
		config,
		repository,
		apiClient,
		(count) => releaseInFlightFn(count),
		logger,
	);

	// Create global buffer
	const buffer = createGlobalBuffer(
		config.globalBufferSize,
		distributor,
		logger,
	);

	// Create poller
	const poller = createOutboxPoller(config, repository, buffer, logger);
	releaseInFlightFn = (count) => poller.releaseInFlight(count);

	// Start all components
	buffer.start();
	poller.start();

	logger.info("Outbox processor started");

	return {
		async stop() {
			logger.info("Stopping outbox processor...");
			poller.stop();
			buffer.stop();
			await repository.close();
			logger.info("Outbox processor stopped");
		},
	};
}
