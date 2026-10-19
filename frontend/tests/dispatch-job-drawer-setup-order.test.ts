import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// An immediate watch runs its callback during setup. DispatchJobDetailDrawer's
// load() resets refs (subscription, connection, audit, ...), so the watch must
// come AFTER every one of them is declared; above them the page crashes with
// "Cannot access 'k' before initialization" and the loading indicator hangs.
// The repo has no component-mounting setup, so this checks the source order.
describe("DispatchJobDetailDrawer setup order", () => {
	const src = readFileSync(new URL("../src/pages/dispatch-jobs/DispatchJobDetailDrawer.vue", import.meta.url), "utf8");

	it("runs the immediate watch after every ref that load() touches is declared", () => {
		const watchAt = src.indexOf("watch(id, load, { immediate: true });");
		expect(watchAt).toBeGreaterThan(-1);
		// What load(), loadRefs() and loadAudit() read or write at call time.
		const touched = ["job", "attempts", "loading", "loadError", "plan", "subscription", "connection", "audit", "auditError", "auditLoading", "auditCollapsed"];
		for (const name of touched) {
			const at = src.search(new RegExp(`^const ${name} = ref\\b`, "m"));
			expect(at, `${name} is declared`).toBeGreaterThan(-1);
			expect(at, `${name} is declared before the immediate watch`).toBeLessThan(watchAt);
		}
	});
});
