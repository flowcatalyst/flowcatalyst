/**
 * Outbox Poller
 *
 * Scheduled service that polls the outbox tables for pending items.
 * Uses a single-poller, status-based architecture with NO row locking.
 *
 * Architecture:
 * 1. Check if sufficient capacity before polling
 * 2. Fetch pending items (simple SELECT)
 * 3. Mark items as in-progress IMMEDIATELY after fetch
 * 4. Add items to global buffer
 */

import type { OutboxItemType } from "./model.js";
import type { OutboxRepository } from "./repository/outbox-repository.js";
import type { GlobalBuffer } from "./global-buffer.js";
import type { OutboxProcessorConfig } from "./env.js";
import type { Logger } from "pino";

const ITEM_TYPES: OutboxItemType[] = ["EVENT", "DISPATCH_JOB", "AUDIT_LOG"];

export interface OutboxPoller {
	/** Release in-flight permits when processing completes. */
	releaseInFlight(count: number): void;
	/** Get current in-flight count. */
	getInFlightCount(): number;
	/** Start polling. */
	start(): void;
	/** Stop polling. */
	stop(): void;
}

export function createOutboxPoller(
	config: OutboxProcessorConfig,
	repository: OutboxRepository,
	buffer: GlobalBuffer,
	logger: Logger,
): OutboxPoller {
	let inFlightCount = 0;
	let polling = false;
	let pollTimer: ReturnType<typeof setInterval> | null = null;
	let recoveryTimer: ReturnType<typeof setInterval> | null = null;

	function releaseInFlight(count: number): void {
		inFlightCount -= count;
		logger.trace(
			{ released: count, remaining: inFlightCount },
			"Released in-flight permits",
		);
	}

	async function doCrashRecovery(): Promise<void> {
		for (const type of ITEM_TYPES) {
			try {
				const stuckItems = await repository.fetchStuckItems(type);
				if (stuckItems.length > 0) {
					const ids = stuckItems.map((item) => String(item.id));
					await repository.resetStuckItems(type, ids);
					logger.info(
						{ count: ids.length, type },
						"Reset stuck items during crash recovery",
					);
				}
			} catch (err) {
				logger.error({ err, type }, "Error during crash recovery");
			}
		}
	}

	async function poll(): Promise<void> {
		if (polling) {
			logger.debug("Previous poll still in progress, skipping");
			return;
		}

		polling = true;
		try {
			// Check capacity before polling
			const availableSlots = config.maxInFlight - inFlightCount;
			if (availableSlots < config.pollBatchSize) {
				logger.debug(
					{ available: availableSlots, needed: config.pollBatchSize },
					"Skipping poll — insufficient capacity",
				);
				return;
			}

			for (const type of ITEM_TYPES) {
				await pollItemType(type);
			}
		} catch (err) {
			logger.error({ err }, "Error during poll cycle");
		} finally {
			polling = false;
		}
	}

	async function pollItemType(type: OutboxItemType): Promise<void> {
		try {
			// 1. Fetch pending items (simple SELECT, no locking)
			const items = await repository.fetchPending(type, config.pollBatchSize);
			if (items.length === 0) return;

			// 2. Mark as in-progress IMMEDIATELY (before buffering)
			const ids = items.map((item) => String(item.id));
			await repository.markAsInProgress(type, ids);

			// 3. Acquire in-flight permits
			inFlightCount += items.length;

			logger.debug(
				{ count: items.length, type },
				"Fetched and marked items as in-progress",
			);

			// 4. Add to buffer
			const rejected = buffer.addAll(items);
			if (rejected > 0) {
				logger.warn(
					{ rejected },
					"Buffer rejected items — items remain in-progress and will be recovered on restart",
				);
			}
		} catch (err) {
			logger.error({ err, type }, "Error polling items");
		}
	}

	async function periodicRecovery(): Promise<void> {
		for (const type of ITEM_TYPES) {
			try {
				const recoverableItems = await repository.fetchRecoverableItems(
					type,
					config.processingTimeoutSeconds,
					config.pollBatchSize,
				);

				if (recoverableItems.length > 0) {
					const ids = recoverableItems.map((item) => String(item.id));
					await repository.resetRecoverableItems(type, ids);
					logger.info(
						{ count: ids.length, type },
						"Periodic recovery: reset items back to PENDING",
					);
				}
			} catch (err) {
				logger.error({ err, type }, "Error during periodic recovery");
			}
		}
	}

	return {
		releaseInFlight,
		getInFlightCount: () => inFlightCount,

		start() {
			// Run crash recovery first, then start polling
			doCrashRecovery()
				.then(() => {
					pollTimer = setInterval(() => {
						poll().catch((err) => {
							logger.error({ err }, "Unhandled error in poll");
						});
					}, config.pollIntervalMs);

					recoveryTimer = setInterval(() => {
						periodicRecovery().catch((err) => {
							logger.error({ err }, "Unhandled error in periodic recovery");
						});
					}, config.recoveryIntervalMs);

					logger.info(
						{
							pollIntervalMs: config.pollIntervalMs,
							recoveryIntervalMs: config.recoveryIntervalMs,
						},
						"OutboxPoller started",
					);
				})
				.catch((err) => {
					logger.error({ err }, "Failed to start OutboxPoller");
				});
		},

		stop() {
			if (pollTimer) {
				clearInterval(pollTimer);
				pollTimer = null;
			}
			if (recoveryTimer) {
				clearInterval(recoveryTimer);
				recoveryTimer = null;
			}
			logger.info("OutboxPoller stopped");
		},
	};
}
