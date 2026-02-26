/**
 * Dispatch Scheduler
 *
 * Polls PENDING dispatch jobs (e.g. manually re-queued via frontend)
 * and publishes them to the message queue, respecting DispatchMode
 * (IMMEDIATE, NEXT_ON_ERROR, BLOCK_ON_ERROR).
 *
 * Components:
 * - PendingJobPoller: queries PENDING jobs, groups by messageGroup, filters by mode
 * - BlockOnErrorChecker: checks for FAILED jobs in message groups
 * - MessageGroupDispatcher: concurrency coordinator with semaphore
 * - MessageGroupQueue: per-group FIFO queue (1 in-flight at a time)
 * - JobDispatcher: publishes to queue, updates status to QUEUED
 * - StaleQueuedJobPoller: safety net for stuck QUEUED jobs
 */

import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { QueuePublisher } from "@flowcatalyst/queue-core";
import {
	type DispatchSchedulerConfig,
	type SchedulerLogger,
	DEFAULT_DISPATCH_SCHEDULER_CONFIG,
} from "./config.js";
import { createBlockOnErrorChecker } from "./block-on-error-checker.js";
import { createJobDispatcher } from "./job-dispatcher.js";
import { createMessageGroupDispatcher } from "./message-group-dispatcher.js";
import { createPendingJobPoller } from "./pending-job-poller.js";
import { createStaleQueuedJobPoller } from "./stale-queued-job-poller.js";

export interface DispatchSchedulerHandle {
	stop(): void;
}

export interface DispatchSchedulerDeps {
	readonly db: PostgresJsDatabase;
	readonly publisher: QueuePublisher;
	readonly logger: SchedulerLogger;
	readonly config?: Partial<DispatchSchedulerConfig> | undefined;
}

export function startDispatchScheduler(
	deps: DispatchSchedulerDeps,
): DispatchSchedulerHandle {
	const config: DispatchSchedulerConfig = {
		...DEFAULT_DISPATCH_SCHEDULER_CONFIG,
		...deps.config,
	};

	const { db, publisher, logger } = deps;

	// Create components
	const blockOnErrorChecker = createBlockOnErrorChecker(db);
	const jobDispatcher = createJobDispatcher(config, db, publisher, logger);
	const groupDispatcher = createMessageGroupDispatcher(
		config,
		jobDispatcher,
		logger,
	);
	const pendingJobPoller = createPendingJobPoller(
		config,
		db,
		blockOnErrorChecker,
		groupDispatcher,
		logger,
	);
	const staleQueuedJobPoller = createStaleQueuedJobPoller(config, db, logger);

	// Start pollers
	pendingJobPoller.start();
	staleQueuedJobPoller.start();

	logger.info("Dispatch Scheduler started");

	return {
		stop() {
			pendingJobPoller.stop();
			staleQueuedJobPoller.stop();
			logger.info("Dispatch Scheduler stopped");
		},
	};
}

// Re-export types
export type { DispatchSchedulerConfig } from "./config.js";
export { DEFAULT_DISPATCH_SCHEDULER_CONFIG } from "./config.js";
