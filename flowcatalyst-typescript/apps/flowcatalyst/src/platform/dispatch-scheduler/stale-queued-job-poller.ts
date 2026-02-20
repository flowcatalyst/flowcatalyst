/**
 * Stale Queued Job Poller
 *
 * Safety net that resets QUEUED dispatch jobs back to PENDING
 * if they have been stuck in QUEUED status beyond a threshold.
 * This handles cases where the queue publish succeeded but the
 * message was lost or never processed.
 */

import { eq, and, lt } from "drizzle-orm";
import { dispatchJobs } from "@flowcatalyst/persistence";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { DispatchSchedulerConfig, SchedulerLogger } from "./config.js";

export interface StaleQueuedJobPoller {
	start(): void;
	stop(): void;
}

export function createStaleQueuedJobPoller(
	config: DispatchSchedulerConfig,
	db: PostgresJsDatabase,
	logger: SchedulerLogger,
): StaleQueuedJobPoller {
	let pollTimer: ReturnType<typeof setInterval> | null = null;

	async function doPoll(): Promise<void> {
		try {
			const thresholdMs = config.staleQueuedThresholdMinutes * 60 * 1000;
			const cutoff = new Date(Date.now() - thresholdMs);

			const result = await db
				.update(dispatchJobs)
				.set({ status: "PENDING", updatedAt: new Date() })
				.where(
					and(
						eq(dispatchJobs.status, "QUEUED"),
						lt(dispatchJobs.updatedAt, cutoff),
					),
				)
				.returning({ id: dispatchJobs.id });

			if (result.length > 0) {
				logger.info(
					{
						count: result.length,
						thresholdMinutes: config.staleQueuedThresholdMinutes,
					},
					"Reset stale QUEUED jobs to PENDING",
				);
			}
		} catch (err) {
			logger.error({ err }, "Error polling for stale QUEUED jobs");
		}
	}

	return {
		start() {
			pollTimer = setInterval(() => {
				doPoll().catch((err) => {
					logger.error({ err }, "Unhandled error in stale QUEUED job poll");
				});
			}, config.staleQueuedPollIntervalMs);

			logger.info(
				{
					staleQueuedPollIntervalMs: config.staleQueuedPollIntervalMs,
					staleQueuedThresholdMinutes: config.staleQueuedThresholdMinutes,
				},
				"StaleQueuedJobPoller started",
			);
		},

		stop() {
			if (pollTimer) {
				clearInterval(pollTimer);
				pollTimer = null;
			}
			logger.info("StaleQueuedJobPoller stopped");
		},
	};
}
