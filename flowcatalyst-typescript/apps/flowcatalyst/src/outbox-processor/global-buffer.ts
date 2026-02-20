/**
 * Global Buffer
 *
 * In-memory bounded buffer between the poller and the group processors.
 * Provides backpressure when processing can't keep up with polling.
 * A drain loop runs on setInterval to route items to the GroupDistributor.
 */

import type { OutboxItem } from "./model.js";
import type { GroupDistributor } from "./group-distributor.js";
import type { Logger } from "pino";

export interface GlobalBuffer {
	/** Add items to the buffer. Returns the number rejected (buffer full). */
	addAll(items: OutboxItem[]): number;
	/** Get the current buffer size. */
	getBufferSize(): number;
	/** Start the drain loop. */
	start(): void;
	/** Stop the drain loop. */
	stop(): void;
}

export function createGlobalBuffer(
	capacity: number,
	distributor: GroupDistributor,
	logger: Logger,
): GlobalBuffer {
	const buffer: OutboxItem[] = [];
	let drainTimer: ReturnType<typeof setInterval> | null = null;

	function addAll(items: OutboxItem[]): number {
		let rejected = 0;
		for (const item of items) {
			if (buffer.length >= capacity) {
				rejected++;
				logger.warn(
					{ itemId: item.id },
					"Buffer full, item will be recovered later",
				);
			} else {
				buffer.push(item);
			}
		}
		return rejected;
	}

	function drain(): void {
		// Drain all available items in a single tick
		while (buffer.length > 0) {
			const item = buffer.shift()!;
			distributor.distribute(item);
		}
	}

	function start(): void {
		// Drain every 10ms — fast enough for throughput, low overhead in Node.js
		drainTimer = setInterval(drain, 10);
		logger.info({ capacity }, "GlobalBuffer started");
	}

	function stop(): void {
		if (drainTimer) {
			clearInterval(drainTimer);
			drainTimer = null;
		}
		// Drain remaining items on shutdown
		drain();
		logger.info("GlobalBuffer stopped");
	}

	return {
		addAll,
		getBufferSize: () => buffer.length,
		start,
		stop,
	};
}
