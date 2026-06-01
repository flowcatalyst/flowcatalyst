import { strict as assert } from "node:assert";
import { describe, it } from "node:test";
import { buildPrincipal } from "../../src/fastify/principal.js";
import { defineRbac } from "../../src/fastify/rbac.js";

function snap(over: Partial<Parameters<typeof buildPrincipal>[0]["snapshot"]> = {}) {
	return {
		id: "prn_x",
		type: "USER" as const,
		scope: "client" as const,
		name: "Test User",
		clients: ["clt_a"],
		roles: ["billing-admin"],
		applications: ["billing"],
		mechanism: "bearer" as const,
		sessionData: {},
		...over,
	};
}

describe("Principal helpers", () => {
	it("hasRole / hasRoles / hasAnyRole", () => {
		const p = buildPrincipal({
			snapshot: snap({ roles: ["a", "b", "c"] }),
			rbac: undefined,
		});
		assert.equal(p.hasRole("a"), true);
		assert.equal(p.hasRole("z"), false);
		assert.equal(p.hasRoles(["a", "b"]), true);
		assert.equal(p.hasRoles(["a", "z"]), false);
		assert.equal(p.hasAnyRole(["a", "z"]), true);
		assert.equal(p.hasAnyRole(["x", "z"]), false);
	});

	it("permissions resolve through RBAC catalogue", () => {
		const rbac = defineRbac()
			.role("billing-admin").grants("invoice:create", "invoice:read")
			.build();
		const p = buildPrincipal({ snapshot: snap(), rbac });
		assert.equal(p.hasPermissionTo(["invoice:read"]), true);
		assert.equal(p.hasPermissionTo(["invoice:create", "invoice:read"]), true);
		assert.equal(p.hasPermissionTo(["invoice:void"]), false);
		assert.equal(p.hasAnyPermissionTo(["invoice:void", "invoice:read"]), true);
	});

	it("permission wildcards match at any segment depth", () => {
		const rbac = defineRbac()
			.role("admin").grants("billing:*")
			.role("super").grants("*")
			.build();
		const admin = buildPrincipal({
			snapshot: snap({ roles: ["admin"] }),
			rbac,
		});
		assert.equal(admin.hasPermissionTo(["billing:invoice:read"]), true);
		assert.equal(admin.hasPermissionTo(["billing:read"]), true);
		assert.equal(admin.hasPermissionTo(["ticket:read"]), false);

		const sup = buildPrincipal({ snapshot: snap({ roles: ["super"] }), rbac });
		assert.equal(sup.hasPermissionTo(["anything:goes:here"]), true);
	});

	it("anchor scope grants implicit cross-client access", () => {
		const p = buildPrincipal({
			snapshot: snap({ scope: "anchor", clients: ["*"] }),
			rbac: undefined,
		});
		assert.equal(p.isAnchor(), true);
		assert.equal(p.canAccessClient("clt_anything"), true);
	});

	it("non-anchor only sees its own clients", () => {
		const p = buildPrincipal({
			snapshot: snap({ clients: ["clt_a", "clt_b"] }),
			rbac: undefined,
		});
		assert.equal(p.isAnchor(), false);
		assert.equal(p.canAccessClient("clt_a"), true);
		assert.equal(p.canAccessClient("clt_x"), false);
	});

	it("permission set is empty when no rbac is provided", () => {
		const p = buildPrincipal({ snapshot: snap(), rbac: undefined });
		assert.equal(p.hasPermissionTo(["anything"]), false);
		assert.equal(p.hasAnyPermissionTo(["a", "b"]), false);
	});
});
