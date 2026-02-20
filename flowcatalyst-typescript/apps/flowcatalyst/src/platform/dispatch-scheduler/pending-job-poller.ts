/**
 * Pending Job Poller
 *
 * Polls for PENDING dispatch jobs and submits them to the MessageGroupDispatcher.
 * Runs on a configurable interval (default 5 seconds).
 *
 * Flow:
 * 1. Query PENDING jobs (batch size)
 * 2. Group by messageGroup
 * 3. Check for blocked groups (BlockOnErrorChecker)
 * 4. Filter by DispatchMode
 * 5. Submit to MessageGroupDispatcher
 */

import { eq, asc } from "drizzle-orm";
import {
	dispatchJobs,
	type DispatchJobRecord,
	type DispatchMode,
} from "@flowcatalyst/persistence";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { BlockOnErrorChecker } from "./block-on-error-checker.js";
import type { MessageGroupDispatcher } from "./message-group-dispatcher.js";
import type { DispatchSchedulerConfig, SchedulerLogger } from "./config.js";

const DEFAULT_MESSAGE_GROUP = "default";

export interface PendingJobPoller {
	start(): void;
	stop(): void;
}

export function createPendingJobPoller(
	config: DispatchSchedulerConfig,
	db: PostgresJsDatabase,
	blockOnErrorChecker: BlockOnErrorChecker,
	groupDispatcher: MessageGroupDispatcher,
	logger: SchedulerLogger,
): PendingJobPoller {
	let pollTimer: ReturnType<typeof setInterval> | null = null;
	let polling = false;

	async function doPoll(): Promise<void> {
		if (polling) return;
		polling = true;

		try {
			// 1. Query PENDING jobs
			const pendingJobs = await db
				.select()
				.from(dispatchJobs)
				.where(eq(dispatchJobs.status, "PENDING"))
				.orderBy(
					asc(dispatchJobs.messageGroup),
					asc(dispatchJobs.sequence),
					asc(dispatchJobs.createdAt),
				)
				.limit(config.batchSize);

			if (pendingJobs.length === 0) return;

			logger.debug(
				{ count: pendingJobs.length },
				"Found pending jobs to process",
			);

			// 2. Group by messageGroup
			const jobsByGroup = new Map<string, DispatchJobRecord[]>();
			for (const job of pendingJobs) {
				const group = job.messageGroup ?? DEFAULT_MESSAGE_GROUP;
				const existing = jobsByGroup.get(group) ?? [];
				existing.push(job);
				jobsByGroup.set(group, existing);
			}

			// 3. Check for blocked groups
			const groupKeys = Array.from(jobsByGroup.keys());
			const blockedGroups =
				await blockOnErrorChecker.getBlockedGroups(groupKeys);

			// 4. Process each group
			for (const [messageGroup, groupJobs] of jobsByGroup) {
				if (blockedGroups.has(messageGroup)) {
					logger.debug(
						{ messageGroup, count: groupJobs.length },
						"Message group is blocked due to FAILED jobs, skipping",
					);
					continue;
				}

				// Filter by DispatchMode
				const dispatchableJobs = filterByDispatchMode(groupJobs, blockedGroups);

				if (dispatchableJobs.length > 0) {
					logger.debug(
						{ messageGroup, count: dispatchableJobs.length },
						"Submitting jobs for message group",
					);
					groupDispatcher.submitJobs(messageGroup, dispatchableJobs);
				}
			}

			// 5. Cleanup empty queues
			groupDispatcher.cleanupEmptyQueues();
		} catch (err) {
			logger.error({ err }, "Error polling for pending jobs");
		} finally {
			polling = false;
		}
	}

	function filterByDispatchMode(
		jobs: DispatchJobRecord[],
		blockedGroups: Set<string>,
	): DispatchJobRecord[] {
		return jobs.filter((job) => {
			const mode: DispatchMode = job.mode ?? "IMMEDIATE";
			const group = job.messageGroup ?? DEFAULT_MESSAGE_GROUP;

			switch (mode) {
				case "IMMEDIATE":
					return true;
				case "NEXT_ON_ERROR":
				case "BLOCK_ON_ERROR":
					return !blockedGroups.has(group);
				default:
					return true;
			}
		});
	}

	return {
		start() {
			pollTimer = setInterval(() => {
				doPoll().catch((err) => {
					logger.error({ err }, "Unhandled error in pending job poll");
				});
			}, config.pollIntervalMs);

			logger.info(
				{ pollIntervalMs: config.pollIntervalMs },
				"PendingJobPoller started",
			);
		},

		stop() {
			if (pollTimer) {
				clearInterval(pollTimer);
				pollTimer = null;
			}
			logger.info("PendingJobPoller stopped");
		},
	};
}
