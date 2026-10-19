/**
 * The dispatch job write actions each have their own permission (they were all
 * dispatch-job:view); the console offers Requeue and Edit payload only to
 * holders, and says what the server's status guard left alone.
 */

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { userHasPermission } from "@/stores/permissions";
import {
	DISPATCH_JOB_CANCEL,
	DISPATCH_JOB_COMPLETE,
	DISPATCH_JOB_EDIT_PAYLOAD,
	DISPATCH_JOB_REDRIVE_COMPLETED,
	DISPATCH_JOB_REQUEUE,
} from "@/utils/dispatch-job-permissions";
import { requeueOutcome, requeueSkipNote } from "@/pages/dispatch-jobs/requeueByFilter";

const src = (rel: string) =>
	readFileSync(fileURLToPath(new URL(`../src/pages/dispatch-jobs/${rel}`, import.meta.url)), "utf8");

describe("permission codes", () => {
	it("are the API's", () => {
		expect(DISPATCH_JOB_REQUEUE).toBe("platform:messaging:dispatch-job:requeue");
		expect(DISPATCH_JOB_REDRIVE_COMPLETED).toBe("platform:messaging:dispatch-job:redrive-completed");
		expect(DISPATCH_JOB_CANCEL).toBe("platform:messaging:dispatch-job:cancel");
		expect(DISPATCH_JOB_COMPLETE).toBe("platform:messaging:dispatch-job:complete");
		expect(DISPATCH_JOB_EDIT_PAYLOAD).toBe("platform:messaging:dispatch-job:edit-payload");
	});

	it("view no longer implies a write", () => {
		const viewer = { permissions: ["platform:messaging:dispatch-job:view"] };
		for (const p of [DISPATCH_JOB_REQUEUE, DISPATCH_JOB_EDIT_PAYLOAD, DISPATCH_JOB_CANCEL, DISPATCH_JOB_COMPLETE]) {
			expect(userHasPermission(viewer, p)).toBe(false);
		}
		expect(userHasPermission({ permissions: ["platform:*:*:*"] }, DISPATCH_JOB_REDRIVE_COMPLETED)).toBe(true);
	});
});

describe("buttons", () => {
	it("Requeue is shown only with the requeue permission", () => {
		const list = src("DispatchJobListPage.vue");
		expect(list).toContain("userHasPermission(authStore.user, DISPATCH_JOB_REQUEUE)");
		expect(list).toMatch(/<Button\s+v-if="canRequeue"\s+:label="requeueSelectedLabel"/);
		expect(list).toMatch(/<SplitButton\s+v-if="canRequeue"/);
		const drawer = src("DispatchJobDetailDrawer.vue");
		expect(drawer).toMatch(/<Button\s+v-if="actions\.requeue"\s+label="Requeue"/);
	});

	it("Edit payload needs the edit-payload permission", () => {
		const drawer = src("DispatchJobDetailDrawer.vue");
		expect(drawer).toContain("userHasPermission(authStore.user, DISPATCH_JOB_EDIT_PAYLOAD)");
		expect(drawer).toContain("canEdit.value && canEditPayload(job.value?.status)");
	});
});

describe("what the status guard left alone", () => {
	it("is empty when nothing was skipped", () => {
		expect(requeueSkipNote({})).toBe("");
		expect(requeueSkipNote({ skippedInFlight: 0, skippedCompleted: 0 })).toBe("");
	});

	it("names in-flight and completed jobs", () => {
		expect(requeueSkipNote({ skippedInFlight: 2 })).toBe(" — 2 jobs in flight (QUEUED or PROCESSING) left alone");
		expect(requeueSkipNote({ skippedCompleted: 1 })).toBe(" — 1 COMPLETED job left alone");
		expect(requeueSkipNote({ skippedInFlight: 1, skippedCompleted: 3 })).toContain("; 3 COMPLETED jobs left alone");
	});

	it("is part of a run's toast", () => {
		const base = {
			requeued: 4, redriven: 0, skippedInFlight: 2, skippedCompleted: 1, truncated: false, remaining: 0,
			window: { from: "2026-10-18T00:00:00Z", to: "2026-10-19T00:00:00Z", defaulted: true },
		};
		expect(requeueOutcome(base).detail).toContain("2 jobs in flight");
		expect(requeueOutcome({ ...base, requeued: 0 }).detail).toContain("1 COMPLETED job left alone");
	});
});
