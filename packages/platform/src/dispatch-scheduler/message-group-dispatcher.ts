/**
 * Message Group Dispatcher
 *
 * Coordinates dispatch across message groups. Maintains in-memory queues
 * per message group and ensures only 1 job per group is dispatched to
 * the external queue at a time. Uses a semaphore to limit concurrent dispatches.
 */

import type { DispatchJobRecord } from "@flowcatalyst/persistence";
import {
	createMessageGroupQueue,
	type MessageGroupQueue,
} from "./message-group-queue.js";
import type { JobDispatcher } from "./job-dispatcher.js";
import type { DispatchSchedulerConfig, SchedulerLogger } from "./config.js";

export interface MessageGroupDispatcher {
	/** Submit jobs for a message group. */
	submitJobs(messageGroup: string, jobs: DispatchJobRecord[]): void;
	/** Clean up empty queues. */
	cleanupEmptyQueues(): void;
	/** Get number of active group queues. */
	getActiveGroupCount(): number;
}

export function createMessageGroupDispatcher(
	config: DispatchSchedulerConfig,
	jobDispatcher: JobDispatcher,
	logger: SchedulerLogger,
): MessageGroupDispatcher {
	const groupQueues = new Map<string, MessageGroupQueue>();

	// Simple semaphore for concurrency limiting
	let activeConcurrency = 0;
	const waitingQueue: Array<() => void> = [];

	function acquireConcurrency(): Promise<void> {
		if (activeConcurrency < config.maxConcurrentGroups) {
			activeConcurrency++;
			return Promise.resolve();
		}
		return new Promise<void>((resolve) => {
			waitingQueue.push(() => {
				activeConcurrency++;
				resolve();
			});
		});
	}

	function releaseConcurrency(): void {
		activeConcurrency--;
		const next = waitingQueue.shift();
		if (next) next();
	}

	async function dispatchJob(job: DispatchJobRecord): Promise<void> {
		await acquireConcurrency();
		try {
			const success = await jobDispatcher.dispatch(job);

			if (success) {
				logger.debug(
					{ jobId: job.id, messageGroup: job.messageGroup },
					"Successfully dispatched job",
				);
			} else {
				logger.warn(
					{ jobId: job.id, messageGroup: job.messageGroup },
					"Failed to dispatch job",
				);
			}

			// Trigger next dispatch in this group
			const queue = groupQueues.get(job.messageGroup ?? "default");
			if (queue) {
				queue.onCurrentJobDispatched();
			}
		} finally {
			releaseConcurrency();
		}
	}

	return {
		submitJobs(messageGroup, jobs) {
			if (jobs.length === 0) return;

			let queue = groupQueues.get(messageGroup);
			if (!queue) {
				queue = createMessageGroupQueue(messageGroup, dispatchJob);
				groupQueues.set(messageGroup, queue);
			}

			queue.addJobs(jobs);
		},

		cleanupEmptyQueues() {
			for (const [key, queue] of groupQueues) {
				if (!queue.hasPendingJobs() && !queue.hasJobInFlight()) {
					groupQueues.delete(key);
				}
			}
		},

		getActiveGroupCount() {
			let count = 0;
			for (const queue of groupQueues.values()) {
				if (queue.hasPendingJobs() || queue.hasJobInFlight()) count++;
			}
			return count;
		},
	};
}
