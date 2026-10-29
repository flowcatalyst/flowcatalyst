/**
 * Definition export / import against the shared fixtures
 * (internal/ui/pages/testdata/definitions/*.json), which the server-rendered
 * UI's tests read too.
 */

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import type { Application } from "@/api/applications";
import type { Connection } from "@/api/connections";
import type { DispatchPool } from "@/api/dispatch-pools";
import type { ServiceAccount } from "@/api/service-accounts";
import type { Subscription } from "@/api/subscriptions";
import {
	applyApplicationDefinition,
	applyConnectionDefinition,
	applyDispatchPoolDefinition,
	applyServiceAccountDefinition,
	applySubscriptionDefinition,
	buildApplicationDefinition,
	buildConnectionDefinition,
	buildDispatchPoolDefinition,
	buildServiceAccountDefinition,
	buildSubscriptionDefinition,
	definitionClientIdentifiers,
	parseDefinition,
	type DefinitionKind,
} from "@/utils/definition-transfer";

function fixture(kind: DefinitionKind): string {
	return readFileSync(
		fileURLToPath(
			new URL(
				`../../internal/ui/pages/testdata/definitions/${kind}.json`,
				import.meta.url,
			),
		),
		"utf8",
	);
}

function definitionOf(kind: DefinitionKind): Record<string, unknown> {
	const parsed = parseDefinition(fixture(kind), kind);
	if (!parsed.ok) throw new Error(parsed.error);
	return parsed.definition;
}

const stamps = {
	createdAt: "2026-01-01T00:00:00Z",
	updatedAt: "2026-01-02T00:00:00Z",
};

const application: Application = {
	...stamps,
	id: "app_1",
	active: true,
	hasLoginClient: false,
	serviceAccountId: "sa_9",
	code: "orders",
	name: "Orders",
	description: "Order capture and fulfilment",
	type: "APPLICATION",
	defaultBaseUrl: "https://orders.qa.example.com",
	iconUrl: "https://orders.qa.example.com/icon.png",
	website: "https://orders.example.com",
	logo: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>',
	logoMimeType: "image/svg+xml",
};

const serviceAccount: ServiceAccount = {
	...stamps,
	id: "sa_1",
	active: true,
	authType: "BEARER",
	principalId: "prn_1",
	oauthClientId: "oc_1",
	clientIds: ["clt_acme"],
	applicationId: "app_1",
	code: "orders-svc",
	name: "Orders service",
	description: "Signs order webhooks",
	scope: "CLIENT",
	roles: ["orders:dispatcher"],
};

const connection: Connection = {
	...stamps,
	id: "con_1",
	source: "UI",
	status: "ACTIVE",
	serviceAccountId: "sa_1",
	clientId: "clt_acme",
	clientIdentifier: "acme",
	code: "billing-hooks",
	name: "Billing hooks",
	description: "Webhooks into billing",
	externalId: "ext-42",
};

const pool: DispatchPool = {
	...stamps,
	id: "pool_1",
	status: "ACTIVE",
	clientId: "clt_acme",
	clientIdentifier: "acme",
	code: "orders-pool",
	name: "Orders pool",
	description: "Order deliveries",
	rateLimit: 600,
	concurrency: 10,
};

const subscription: Subscription = {
	...stamps,
	id: "sub_1",
	source: "UI",
	status: "ACTIVE",
	dataOnly: false,
	maxRetries: 3,
	customConfig: [{ key: "k", value: "v" }],
	serviceAccountId: "sa_1",
	createdBy: "prn_1",
	code: "orders-hook",
	applicationCode: "orders",
	name: "Orders hook",
	description: "Order events to billing",
	endpoint: "https://billing.example.com/hooks/orders",
	clientScoped: true,
	clientId: "clt_acme",
	clientIdentifier: "acme",
	eventTypes: [
		{
			eventTypeId: "et_1",
			eventTypeCode: "orders:sales:order:placed",
			specVersion: "1.0",
		},
	],
	connectionId: "con_1",
	queue: "DEFAULT",
	dispatchPoolId: "pool_1",
	dispatchPoolCode: "orders-pool",
	mode: "NEXT_ON_ERROR",
	maxAgeSeconds: 86400,
	timeoutSeconds: 30,
	delaySeconds: 0,
	sequence: 99,
};

const clients = [{ id: "clt_acme", identifier: "acme" }];
const eventTypes = [
	{
		id: "et_1",
		code: "orders:sales:order:placed",
		clientScoped: true,
		currentVersion: "1.0",
	},
];

describe("export builds exactly the fixture text", () => {
	it("application", () => {
		const out = buildApplicationDefinition(application);
		expect(out.text).toBe(fixture("application"));
		expect(out.filename).toBe("application-orders.json");
		expect(out.notes).toEqual([]);
	});

	it("service-account", () => {
		const out = buildServiceAccountDefinition(serviceAccount, {
			clients,
			applicationCode: "orders",
		});
		expect(out.text).toBe(fixture("service-account"));
		expect(out.filename).toBe("service-account-orders-svc.json");
		expect(out.notes).toEqual([]);
	});

	it("connection", () => {
		const out = buildConnectionDefinition(connection, {
			serviceAccountCode: "orders-svc",
		});
		expect(out.text).toBe(fixture("connection"));
		expect(out.filename).toBe("connection-billing-hooks.json");
		expect(out.notes).toEqual([]);
	});

	it("dispatch-pool", () => {
		const out = buildDispatchPoolDefinition(pool);
		expect(out.text).toBe(fixture("dispatch-pool"));
		expect(out.filename).toBe("dispatch-pool-orders-pool.json");
		expect(out.notes).toEqual([]);
	});

	it("subscription", () => {
		const out = buildSubscriptionDefinition(subscription, {
			connectionCode: "billing-hooks",
		});
		expect(out.text).toBe(fixture("subscription"));
		expect(out.filename).toBe("subscription-orders-hook.json");
		expect(out.notes).toEqual([]);
	});
});

describe("export omits what the viewer cannot read, and says so", () => {
	it("never writes an id in place of a code", () => {
		const sa = buildServiceAccountDefinition(serviceAccount, {});
		expect(sa.text).not.toContain("clientIdentifiers");
		expect(sa.text).not.toContain("applicationCode");
		expect(sa.text).not.toContain("clt_acme");
		expect(sa.text).not.toContain("app_1");
		expect(sa.notes).toEqual([
			"Client identifiers not included: you can't read clients.",
			"Application not included: you can't read applications.",
		]);

		const conn = buildConnectionDefinition(connection, {});
		expect(conn.text).not.toContain("serviceAccountCode");
		expect(conn.text).not.toContain("sa_1");
		expect(conn.notes).toEqual([
			"Service account not included: you can't read service accounts.",
		]);

		const sub = buildSubscriptionDefinition(subscription, {});
		expect(sub.text).not.toContain("connectionCode");
		expect(sub.text).not.toContain("con_1");
		expect(sub.notes).toEqual([
			"Connection not included: you can't read connections.",
		]);
	});

	it("omits unset keys and a concurrency-only pool's rateLimit", () => {
		const { rateLimit: _omit, description: _d, ...bare } = pool;
		const out = buildDispatchPoolDefinition({
			...bare,
			clientId: undefined,
			clientIdentifier: undefined,
		});
		expect(out.text).not.toContain("rateLimit");
		expect(out.text).not.toContain("description");
		expect(out.text).not.toContain("clientIdentifier");
		expect(out.text.endsWith("}\n")).toBe(true);
	});
});

describe("import resolves the fixtures with no notices", () => {
	it("application", () => {
		const { values, notices } = applyApplicationDefinition(
			definitionOf("application"),
		);
		expect(notices).toEqual([]);
		expect(values).toMatchObject({
			code: "orders",
			type: "APPLICATION",
			logoMimeType: "image/svg+xml",
		});
	});

	it("service-account (roles and application are reported, not applied)", () => {
		const def = definitionOf("service-account");
		const { values, notices } = applyServiceAccountDefinition(def, { clients });
		expect(values.selectedClientIds).toEqual(["clt_acme"]);
		expect(values.scope).toBe("CLIENT");
		expect(notices).toEqual([
			'Not applied by this form: applicationCode "orders".',
			"Assign roles after creating: orders:dispatcher",
		]);
	});

	it("connection", () => {
		const { values, notices } = applyConnectionDefinition(
			definitionOf("connection"),
			{
				serviceAccounts: [{ id: "sa_1", code: "orders-svc" }],
				clients,
			},
		);
		expect(notices).toEqual([]);
		expect(values.serviceAccountId).toBe("sa_1");
		expect(values.clientId).toBe("clt_acme");
	});

	it("dispatch-pool", () => {
		const { values, notices } = applyDispatchPoolDefinition(
			definitionOf("dispatch-pool"),
			{ clients },
		);
		expect(notices).toEqual([]);
		expect(values).toMatchObject({
			rateLimit: 600,
			concurrency: 10,
			clientId: "clt_acme",
		});
	});

	it("subscription", () => {
		const { values, notices } = applySubscriptionDefinition(
			definitionOf("subscription"),
			{
				connections: [{ id: "con_1", code: "billing-hooks" }],
				dispatchPools: [{ id: "pool_1", code: "orders-pool" }],
				eventTypes,
				clients,
			},
		);
		expect(notices).toEqual([
			'Not applied by this form: applicationCode "orders".',
		]);
		expect(values).toMatchObject({
			connectionId: "con_1",
			dispatchPoolId: "pool_1",
			selectedEventTypes: ["et_1"],
			clientId: "clt_acme",
			clientScoped: true,
			delaySeconds: 0,
			sequence: 99,
		});
	});
});

describe("import lists every unresolved reference", () => {
	it("service-account", () => {
		const { values, notices } = applyServiceAccountDefinition(
			definitionOf("service-account"),
			{ clients: [] },
		);
		expect(values.selectedClientIds).toBeUndefined();
		expect(notices[0]).toContain('Client "acme" was not found here');
	});

	it("connection", () => {
		const { values, notices } = applyConnectionDefinition(
			definitionOf("connection"),
			{ serviceAccounts: [], clients: [] },
		);
		expect(values.serviceAccountId).toBeUndefined();
		expect(values.clientId).toBeUndefined();
		expect(notices).toEqual([
			'Service account "orders-svc" was not found here. Import or create it first, then choose it.',
			'Client "acme" was not found here (or you can\'t access it). Import or create it first, then choose it.',
		]);
	});

	it("dispatch-pool", () => {
		const { notices } = applyDispatchPoolDefinition(
			definitionOf("dispatch-pool"),
			{ clients: [] },
		);
		expect(notices).toHaveLength(1);
		expect(notices[0]).toContain('Client "acme"');
	});

	it("subscription", () => {
		const { values, notices } = applySubscriptionDefinition(
			definitionOf("subscription"),
			{ connections: [], dispatchPools: [], eventTypes: [], clients: [] },
		);
		expect(values.connectionId).toBeUndefined();
		expect(values.dispatchPoolId).toBeUndefined();
		expect(values.selectedEventTypes).toBeUndefined();
		expect(values.clientId).toBeUndefined();
		const text = notices.join("\n");
		expect(text).toContain('Connection "billing-hooks" was not found here');
		expect(text).toContain('Dispatch pool "orders-pool" was not found here');
		expect(text).toContain(
			'Event type "orders:sales:order:placed" was not found here',
		);
		expect(text).toContain('Client "acme" was not found here');
	});

	it("subscription: an event type of the wrong client scoping or version is reported", () => {
		const { values, notices } = applySubscriptionDefinition(
			definitionOf("subscription"),
			{
				connections: [],
				dispatchPools: [],
				eventTypes: [{ ...eventTypes[0]!, currentVersion: "2.0" }],
				clients,
			},
		);
		expect(values.selectedEventTypes).toEqual(["et_1"]);
		expect(notices.join("\n")).toContain("version 1.0 is not the current version (2.0)");

		const mismatch = applySubscriptionDefinition(definitionOf("subscription"), {
			connections: [],
			dispatchPools: [],
			eventTypes: [{ ...eventTypes[0]!, clientScoped: false }],
			clients,
		});
		expect(mismatch.values.selectedEventTypes).toBeUndefined();
		expect(mismatch.notices.join("\n")).toContain("does not match");
	});
});

describe("import reports unknown keys and wrong-typed values", () => {
	it("lists unknown keys as ignored", () => {
		const { values, notices } = applyApplicationDefinition({
			code: "x",
			colour: "red",
			extra: 1,
		});
		expect(values).toEqual({ code: "x" });
		expect(notices).toEqual(["Ignored (unknown keys): colour, extra."]);
	});

	it("does not apply a wrong-typed value silently", () => {
		const { values, notices } = applyDispatchPoolDefinition(
			{ code: "p", concurrency: "ten", rateLimit: 5 },
			{ clients },
		);
		expect(values).toEqual({ code: "p", rateLimit: 5 });
		expect(notices).toEqual([
			'Not applied by this form: concurrency should be a number (got "ten").',
		]);
	});

	it("names the client identifiers to look up", () => {
		expect(definitionClientIdentifiers(definitionOf("service-account"))).toEqual([
			"acme",
		]);
		expect(definitionClientIdentifiers(definitionOf("dispatch-pool"))).toEqual([
			"acme",
		]);
		expect(definitionClientIdentifiers(definitionOf("application"))).toEqual([]);
	});
});

describe("parseDefinition refuses what it cannot read", () => {
	it("accepts each fixture for its own kind", () => {
		for (const kind of [
			"application",
			"service-account",
			"connection",
			"dispatch-pool",
			"subscription",
		] as const) {
			expect(parseDefinition(fixture(kind), kind).ok).toBe(true);
		}
	});

	it("refuses another kind", () => {
		const out = parseDefinition(fixture("connection"), "dispatch-pool");
		expect(out).toEqual({
			ok: false,
			error: "This is a connection definition; this form takes a dispatch-pool definition.",
		});
	});

	it("refuses an unknown version", () => {
		const text = fixture("connection").replace('"version": 1', '"version": 2');
		const out = parseDefinition(text, "connection");
		expect(out.ok).toBe(false);
		if (!out.ok) expect(out.error).toContain("Unsupported definition version 2");
	});

	it("refuses text that is not JSON", () => {
		expect(parseDefinition("{ nope", "connection")).toEqual({
			ok: false,
			error: "This is not valid JSON.",
		});
	});

	it("refuses JSON that is not a definition envelope", () => {
		expect(parseDefinition('{"a":1}', "connection").ok).toBe(false);
		expect(parseDefinition("[]", "connection").ok).toBe(false);
		const noBody = '{"flowcatalyst":"definition","kind":"connection","version":1}';
		expect(parseDefinition(noBody, "connection")).toEqual({
			ok: false,
			error: "The definition is missing.",
		});
	});
});
