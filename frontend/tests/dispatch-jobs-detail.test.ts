import { describe, expect, it } from "vitest";
import { userHasPermission } from "@/stores/permissions";
import { jobActions } from "@/utils/dispatch-job-permissions";
import { auditActor, auditOperationLabel, explainJobActionError } from "@/pages/dispatch-jobs/jobDetail";

const as = (...perms: string[]) => (p: string) => userHasPermission({ permissions: perms }, p);
const P = "platform:messaging:dispatch-job:";

describe("jobActions", () => {
	it("a view-only user gets no write action", () => {
		for (const s of ["PENDING", "FAILED", "COMPLETED"]) {
			expect(jobActions(s, as(`${P}view`))).toEqual({ requeue: false, redrive: false, cancel: false, complete: false });
		}
	});
	it("requeue needs its permission and a resettable status", () => {
		const has = as(`${P}requeue`);
		for (const s of ["PENDING", "FAILED", "ERROR", "CANCELLED", "EXPIRED"]) expect(jobActions(s, has).requeue).toBe(true);
		for (const s of ["QUEUED", "PROCESSING", "COMPLETED"]) expect(jobActions(s, has).requeue).toBe(false);
	});
	it("redrive needs requeue AND redrive-completed, on a COMPLETED job", () => {
		expect(jobActions("COMPLETED", as(`${P}requeue`, `${P}redrive-completed`)).redrive).toBe(true);
		expect(jobActions("COMPLETED", as(`${P}redrive-completed`)).redrive).toBe(false);
		expect(jobActions("COMPLETED", as(`${P}requeue`)).redrive).toBe(false);
		expect(jobActions("FAILED", as(`${P}requeue`, `${P}redrive-completed`)).redrive).toBe(false);
	});
	it("cancel is for unfinished and FAILED jobs, complete for FAILED ones, each with its own permission", () => {
		expect(jobActions("FAILED", as(`${P}cancel`))).toMatchObject({ cancel: true, complete: false });
		expect(jobActions("FAILED", as(`${P}complete`))).toMatchObject({ cancel: false, complete: true });
		expect(jobActions("PENDING", as(`${P}cancel`, `${P}complete`))).toMatchObject({ cancel: true, complete: false });
		for (const s of ["ERROR", "QUEUED", "PROCESSING", "IN_PROGRESS"]) expect(jobActions(s, as(`${P}cancel`)).cancel).toBe(true);
		for (const s of ["COMPLETED", "CANCELLED", "EXPIRED"]) {
			expect(jobActions(s, as(`${P}cancel`)).cancel).toBe(false);
		}
	});
	it("a platform wildcard holder gets all of them", () => {
		expect(jobActions("FAILED", as("platform:*:*:*"))).toMatchObject({ requeue: true, cancel: true, complete: true });
	});
});

describe("audit rows", () => {
	it("label the command", () => {
		expect(auditOperationLabel("CancelCommand")).toBe("Cancel");
		expect(auditOperationLabel("EditPayloadCommand")).toBe("Edit payload");
	});
	it("name who did it", () => {
		expect(auditActor({ principalName: "Ann", principalId: "p1" })).toBe("Ann");
		expect(auditActor({ principalId: "p1" })).toBe("p1");
		expect(auditActor({})).toBe("system");
	});
});

describe("cancel / complete errors", () => {
	it("explains the 409", () => {
		expect(explainJobActionError("cancel", { status: 409 })).toMatch(/finished meanwhile/);
		expect(explainJobActionError("complete", { status: 409 })).toMatch(/no longer FAILED/);
		expect(explainJobActionError("complete", { status: 500, message: "boom" })).toBe("boom");
	});
});
