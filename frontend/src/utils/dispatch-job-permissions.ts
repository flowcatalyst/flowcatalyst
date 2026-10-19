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
/** Cancel a FAILED job. */
export const DISPATCH_JOB_CANCEL = "platform:messaging:dispatch-job:cancel";
/** Mark a FAILED job COMPLETED (delivered out of band). */
export const DISPATCH_JOB_COMPLETE = "platform:messaging:dispatch-job:complete";
/** Edit the payload of a PENDING or FAILED job. */
export const DISPATCH_JOB_EDIT_PAYLOAD = "platform:messaging:dispatch-job:edit-payload";
