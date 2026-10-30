/** Small pure helpers behind the dispatch job drawer (tests/dispatch-jobs-detail.test.ts). */

/** The audit log is read with this permission (GET /api/audit-logs/...). */
export const AUDIT_LOG_VIEW = "platform:admin:audit-log:view";

/**
 * The audit entity type of a dispatch job: its domain events use the subject
 * platform.dispatchjob.<id>, which the audit writer capitalises.
 */
export const DISPATCH_JOB_AUDIT_ENTITY = "Dispatchjob";

/** "CancelCommand" -> "Cancel", "EditPayloadCommand" -> "Edit payload". */
export function auditOperationLabel(operation: string): string {
	const base = operation.replace(/Command$/, "");
	const words = base.replace(/([a-z0-9])([A-Z])/g, "$1 $2").toLowerCase();
	return words ? words.charAt(0).toUpperCase() + words.slice(1) : operation;
}

/** Who did it: the name if the audit row has one, else the principal id, else the system. */
export function auditActor(row: { principalName?: string; principalId?: string }): string {
	return row.principalName || row.principalId || "system";
}

/** The 409 the cancel/complete actions answer when the job is no longer FAILED. */
export function explainJobActionError(action: "cancel" | "complete", err: unknown): string {
	const e = err as { status?: number; message?: string } | null;
	if (e?.status === 409) {
		return action === "cancel"
			? "The job has finished meanwhile, so it cannot be cancelled. It has been reloaded."
			: "The job is no longer FAILED, so it cannot be marked completed. It has been reloaded.";
	}
	return e?.message || `The job could not be ${action === "cancel" ? "cancelled" : "completed"}.`;
}
