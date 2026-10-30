/**
 * Permissions the dispatch job pages check before offering an action the
 * API would refuse (internal/platform/dispatchjob/api: viewRawPerm).
 */

/** Sign (POST /api/dispatch-jobs/{id}/sign) and the raw views need this. */
export const DISPATCH_JOB_VIEW_RAW = "platform:messaging:dispatch-job:view-raw";

/**
 * The write actions each have their own permission (they were all
 * dispatch-job:view before the split; the API checks the same codes).
 */
/** Requeue by ids, by filter and its preview. */
export const DISPATCH_JOB_REQUEUE = "platform:messaging:dispatch-job:requeue";
/** Re-delivering COMPLETED jobs (a requeue that also sets redriveCompleted). */
export const DISPATCH_JOB_REDRIVE_COMPLETED = "platform:messaging:dispatch-job:redrive-completed";
/** Cancel (ignore) a PENDING or FAILED job. */
export const DISPATCH_JOB_CANCEL = "platform:messaging:dispatch-job:cancel";
/** Mark a FAILED job COMPLETED (delivered out of band). */
export const DISPATCH_JOB_COMPLETE = "platform:messaging:dispatch-job:complete";
/** Edit the payload of a PENDING or FAILED job. */
export const DISPATCH_JOB_EDIT_PAYLOAD = "platform:messaging:dispatch-job:edit-payload";

/** Statuses a requeue resets (the server's guard: PENDING, FAILED/ERROR, CANCELLED, EXPIRED). */
const REQUEUEABLE = new Set(["PENDING", "FAILED", "ERROR", "CANCELLED", "EXPIRED"]);

/** Statuses a cancel accepts (the server's guard: PENDING, FAILED/ERROR). */
const CANCELLABLE = new Set(["PENDING", "FAILED", "ERROR"]);

export interface JobActions {
	/** Reset to PENDING (not a COMPLETED or in-flight job). */
	requeue: boolean;
	/** Re-deliver a COMPLETED job: needs requeue AND redrive-completed. */
	redrive: boolean;
	/** Cancel (ignore) a PENDING or FAILED job. */
	cancel: boolean;
	/** Mark a FAILED job COMPLETED (delivered out of band). */
	complete: boolean;
}

/**
 * The write actions to offer for a job: its status says what the API would
 * accept, the caller's permissions say what it would allow. Anything the
 * user lacks permission for is hidden, not disabled.
 */
export function jobActions(status: string | undefined, has: (permission: string) => boolean): JobActions {
	const s = status ?? "";
	const canRequeue = has(DISPATCH_JOB_REQUEUE);
	return {
		requeue: canRequeue && REQUEUEABLE.has(s),
		redrive: canRequeue && has(DISPATCH_JOB_REDRIVE_COMPLETED) && s === "COMPLETED",
		cancel: has(DISPATCH_JOB_CANCEL) && CANCELLABLE.has(s),
		complete: has(DISPATCH_JOB_COMPLETE) && s === "FAILED",
	};
}
