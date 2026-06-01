import { strict as assert } from "node:assert";
import { describe, it } from "node:test";
import { defineRbac } from "../../src/fastify/rbac.js";

describe("RBAC catalogue", () => {
	it("unions permissions across roles", () => {
		const rbac = defineRbac()
			.role("a").grants("p1", "p2")
			.role("b").grants("p2", "p3")
			.build();
		assert.deepEqual([...rbac.resolve(["a", "b"])].sort(), ["p1", "p2", "p3"]);
	});

	it("ignores unknown roles silently", () => {
		const rbac = defineRbac().role("a").grants("p1").build();
		assert.deepEqual(rbac.resolve(["a", "ghost"]), ["p1"]);
	});

	it("multiple grants on the same role accumulate", () => {
		const rbac = defineRbac()
			.role("a").grants("p1", "p2")
			.role("a").grants("p3")
			.build();
		assert.deepEqual([...rbac.resolve(["a"])].sort(), ["p1", "p2", "p3"]);
	});

	it("rejects empty role name and empty permission", () => {
		const builder = defineRbac();
		assert.throws(() => builder.role(""));
		assert.throws(() => builder.role("a").grants(""));
	});

	it("returns empty array when no roles supplied", () => {
		const rbac = defineRbac().role("a").grants("p1").build();
		assert.deepEqual(rbac.resolve([]), []);
	});

	it("frozen catalogue is decoupled from builder mutations", () => {
		const builder = defineRbac().role("a").grants("p1");
		const cat1 = builder.build();
		builder.role("a").grants("p2");
		// Already-built catalogue should not see the later grant.
		assert.deepEqual(cat1.resolve(["a"]), ["p1"]);
	});
});
