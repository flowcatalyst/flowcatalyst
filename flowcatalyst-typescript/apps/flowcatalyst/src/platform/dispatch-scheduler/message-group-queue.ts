/**
 * Message Group Queue
 *
 * In-memory queue for a single message group.
 * Ensures only 1 job is dispatched to the external queue at a time per group.
 * Jobs are sorted by sequence number and creation time for consistent ordering.
 */

import type { DispatchJobRecord } from "@flowcatalyst/persistence";

export interface MessageGroupQueue {
	/** Add jobs to this queue (sorted by sequence, createdAt). */
	addJobs(jobs: DispatchJobRecord[]): void;
	/** Called when the current in-flight job has been dispatched. Triggers next. */
	onCurrentJobDispatched(): void;
	/** Whether there are pending jobs. */
	hasPendingJobs(): boolean;
	/** Get count of pending jobs. */
	getPendingCount(): number;
	/** Whether a job is currently in flight. */
	hasJobInFlight(): boolean;
}

export function createMessageGroupQueue(
	messageGroup: string,
	dispatchFn: (job: DispatchJobRecord) => Promise<void>,
): MessageGroupQueue {
	const pendingJobs: DispatchJobRecord[] = [];
	let jobInFlight = false;

	function addJobs(jobs: DispatchJobRecord[]): void {
		// Sort by sequence then by createdAt
		const sorted = [...jobs].sort((a, b) => {
			const seqCompare = (a.sequence ?? 99) - (b.sequence ?? 99);
			if (seqCompare !== 0) return seqCompare;
			return (a.createdAt?.getTime() ?? 0) - (b.createdAt?.getTime() ?? 0);
		});

		pendingJobs.push(...sorted);
		tryDispatchNext();
	}

	function onCurrentJobDispatched(): void {
		jobInFlight = false;
		tryDispatchNext();
	}

	function tryDispatchNext(): void {
		if (jobInFlight) return;

		const next = pendingJobs.shift();
		if (!next) return;

		jobInFlight = true;

		dispatchFn(next).catch(() => {
			// On failure, release in-flight flag so next job can be processed
			onCurrentJobDispatched();
		});
	}

	return {
		addJobs,
		onCurrentJobDispatched,
		hasPendingJobs: () => pendingJobs.length > 0,
		getPendingCount: () => pendingJobs.length,
		hasJobInFlight: () => jobInFlight,
	};
}
