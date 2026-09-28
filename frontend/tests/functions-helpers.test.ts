/**
 * Pure-logic tests for the Functions admin pages
 * (src/pages/functions/functionHelpers.ts): address/name validation,
 * describe (fc_describe JSON) parsing, and missing-settings computation.
 * No @vue/test-utils in this repo — pages keep their non-reactive logic in
 * a plain module precisely so it can be pinned like this.
 */

import { describe, expect, it } from "vitest";
import {
	aliasesForVersion,
	buildFunctionAddress,
	computeMissingSettings,
	describeFailure,
	describeWiring,
	hasMissingSettings,
	isValidFunctionName,
	liveOrNewestVersion,
	parseDescribe,
	versionDigestShort,
} from "@/pages/functions/functionHelpers";
import type { FunctionAlias, FunctionSetting, FunctionVersion } from "@/api/functions";

describe("isValidFunctionName", () => {
	it("accepts a lowercase name starting with a letter", () => {
		expect(isValidFunctionName("invoice-pdf")).toBe(true);
		expect(isValidFunctionName("a")).toBe(true);
		expect(isValidFunctionName("a1-b2")).toBe(true);
	});
	it("rejects a name starting with a digit or hyphen", () => {
		expect(isValidFunctionName("1abc")).toBe(false);
		expect(isValidFunctionName("-abc")).toBe(false);
	});
	it("rejects uppercase and other characters", () => {
		expect(isValidFunctionName("Invoice")).toBe(false);
		expect(isValidFunctionName("invoice_pdf")).toBe(false);
		expect(isValidFunctionName("invoice.pdf")).toBe(false);
	});
	it("rejects a name over 63 characters", () => {
		expect(isValidFunctionName("a" + "b".repeat(62))).toBe(true); // 63 chars: ok
		expect(isValidFunctionName("a" + "b".repeat(63))).toBe(false); // 64 chars: too long
	});
	it("rejects an empty name", () => {
		expect(isValidFunctionName("")).toBe(false);
	});
});

describe("buildFunctionAddress", () => {
	it("joins application code and name with a dot", () => {
		expect(buildFunctionAddress("billing", "invoice-pdf")).toBe("billing.invoice-pdf");
	});
	it("is empty until both sides are known", () => {
		expect(buildFunctionAddress(null, "invoice-pdf")).toBe("");
		expect(buildFunctionAddress("billing", "")).toBe("");
		expect(buildFunctionAddress(undefined, undefined)).toBe("");
	});
});

describe("parseDescribe", () => {
	it("extracts endpoints, subscriptions, schedules and declared keys", () => {
		const raw = {
			abi: 1,
			endpoints: [
				{ method: "POST", path: "/events/order-created", auth: "webhook" },
				{ method: "GET", path: "/api/orders/{id}", auth: "platform" },
			],
			subscriptions: [
				{ eventType: "orders:order:order:created", path: "/events/order-created", mode: "IMMEDIATE" },
			],
			schedules: [{ cron: "*/5 * * * *", path: "/events/tick" }],
			config: ["GREETING"],
			secrets: ["STRIPE_KEY"],
			db: ["main"],
			httpAllow: ["api.stripe.com"],
			emits: ["orders:order:order:shipped"],
		};
		const d = parseDescribe(raw);
		expect(d.abi).toBe(1);
		expect(d.endpoints).toHaveLength(2);
		expect(d.endpoints[0]).toEqual({
			method: "POST",
			path: "/events/order-created",
			auth: "webhook",
			maxBodyBytes: undefined,
		});
		expect(d.subscriptions[0]?.eventType).toBe("orders:order:order:created");
		expect(d.schedules[0]?.cron).toBe("*/5 * * * *");
		expect(d.config).toEqual(["GREETING"]);
		expect(d.secrets).toEqual(["STRIPE_KEY"]);
		expect(d.db).toEqual(["main"]);
		expect(d.httpAllow).toEqual(["api.stripe.com"]);
		expect(d.emits).toEqual(["orders:order:order:shipped"]);
	});

	it("never throws on malformed or absent input — a FAILED version may carry none", () => {
		expect(parseDescribe(null)).toEqual({
			endpoints: [],
			subscriptions: [],
			schedules: [],
			config: [],
			secrets: [],
			db: [],
			httpAllow: [],
			emits: [],
		});
		expect(parseDescribe(undefined).endpoints).toEqual([]);
		expect(parseDescribe("not an object").config).toEqual([]);
		expect(parseDescribe({ endpoints: "not an array" }).endpoints).toEqual([]);
		expect(parseDescribe({ config: [1, 2, "GREETING"] }).config).toEqual(["GREETING"]);
	});
});

describe("describeFailure", () => {
	it("returns null for no failure", () => {
		expect(describeFailure(null)).toBeNull();
		expect(describeFailure(undefined)).toBeNull();
	});
	it("prefers a message field on an object failure", () => {
		expect(describeFailure({ message: "bad import", code: "LOAD:IMPORT_NOT_ALLOWED" })).toBe(
			"bad import",
		);
	});
	it("falls back to a reason field", () => {
		expect(describeFailure({ reason: "compile error" })).toBe("compile error");
	});
	it("passes through a plain string", () => {
		expect(describeFailure("boom")).toBe("boom");
	});
});

describe("liveOrNewestVersion", () => {
	const versions: FunctionVersion[] = [
		{ abi: 1, describe: {}, digest: "d1", number: 1, functionId: "fn_1", status: "RETIRED", runtime: "wasm" } as FunctionVersion,
		{ abi: 1, describe: {}, digest: "d2", number: 2, functionId: "fn_1", status: "READY", runtime: "wasm" } as FunctionVersion,
		{ abi: 1, describe: {}, digest: "d3", number: 3, functionId: "fn_1", status: "READY", runtime: "wasm" } as FunctionVersion,
	];

	it("prefers the version the live alias points at", () => {
		const aliases: FunctionAlias[] = [
			{ functionId: "fn_1", name: "live", version: 2, updatedAt: "" },
		];
		expect(liveOrNewestVersion(versions, aliases)?.number).toBe(2);
	});

	it("falls back to the newest version when there is no live alias", () => {
		expect(liveOrNewestVersion(versions, [])?.number).toBe(3);
	});

	it("returns null with no versions", () => {
		expect(liveOrNewestVersion([], [])).toBeNull();
	});
});

describe("aliasesForVersion", () => {
	it("lists alias names pointing at a version, sorted", () => {
		const aliases: FunctionAlias[] = [
			{ functionId: "fn_1", name: "qa", version: 3, updatedAt: "" },
			{ functionId: "fn_1", name: "live", version: 3, updatedAt: "" },
			{ functionId: "fn_1", name: "canary", version: 2, updatedAt: "" },
		];
		expect(aliasesForVersion(aliases, 3)).toEqual(["live", "qa"]);
		expect(aliasesForVersion(aliases, 2)).toEqual(["canary"]);
		expect(aliasesForVersion(aliases, 99)).toEqual([]);
	});
});

describe("versionDigestShort", () => {
	it("truncates a long digest", () => {
		const digest = "a".repeat(64);
		expect(versionDigestShort(digest)).toBe("aaaaaaaaaaaa…");
	});
	it("leaves a short digest alone", () => {
		expect(versionDigestShort("short")).toBe("short");
	});
});

describe("computeMissingSettings / hasMissingSettings", () => {
	const declared = { config: ["GREETING"], secrets: ["STRIPE_KEY"], db: ["main"] };

	it("flags declared keys with no settings row", () => {
		const settings: FunctionSetting[] = [];
		const missing = computeMissingSettings(declared, settings);
		expect(missing).toEqual({ config: ["GREETING"], secrets: ["STRIPE_KEY"], db: ["main"] });
		expect(hasMissingSettings(missing)).toBe(true);
	});

	it("clears a key once its kind has a settings row — value absent for secret/db", () => {
		const settings: FunctionSetting[] = [
			{ key: "GREETING", kind: "CONFIG", value: "Hi", updatedAt: "" },
			{ key: "STRIPE_KEY", kind: "SECRET", updatedAt: "" }, // no `value` — write-only
			{ key: "main", kind: "DB", updatedAt: "" },
		];
		const missing = computeMissingSettings(declared, settings);
		expect(missing).toEqual({ config: [], secrets: [], db: [] });
		expect(hasMissingSettings(missing)).toBe(false);
	});

	it("does not cross kinds — a config row named like a secret key doesn't satisfy it", () => {
		const settings: FunctionSetting[] = [
			{ key: "STRIPE_KEY", kind: "CONFIG", value: "not-actually-a-secret", updatedAt: "" },
		];
		const missing = computeMissingSettings(declared, settings);
		expect(missing.secrets).toEqual(["STRIPE_KEY"]);
	});
});

describe("describeWiring", () => {
	it("summarises pool and non-zero counts", () => {
		expect(
			describeWiring({
				dispatchPoolCode: "fn-billing-invoice-pdf",
				subscriptionsCreated: 2,
				subscriptionsUpdated: 0,
				schedulesCreated: 1,
			}),
		).toBe("pool fn-billing-invoice-pdf · 2 subscriptions created · 1 schedule created");
	});
	it("singularises a count of one", () => {
		expect(describeWiring({ subscriptionsCreated: 1 })).toBe("1 subscription created");
	});
	it("returns null for nothing to report", () => {
		expect(describeWiring(null)).toBeNull();
		expect(describeWiring(undefined)).toBeNull();
		expect(describeWiring({})).toBeNull();
	});
});
