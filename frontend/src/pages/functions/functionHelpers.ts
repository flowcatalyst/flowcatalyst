/**
 * Pure helpers for the Functions pages: address/name validation, describe
 * (fc_describe JSON, docs/function-runner-plan.md §5.4) parsing, and the
 * settings/wiring/version-display bits that don't need Vue reactivity.
 * Kept framework-free so they're unit-testable without mounting components
 * (this repo has no @vue/test-utils; see tests/functions-helpers.test.ts).
 */

import type { FunctionAlias, FunctionSetting, FunctionVersion, FunctionWiring } from "@/api/functions";

// §4: "name matches ^[a-z][a-z0-9-]{0,62}$"
export const FUNCTION_NAME_PATTERN = /^[a-z][a-z0-9-]{0,62}$/;

export function isValidFunctionName(name: string): boolean {
	return FUNCTION_NAME_PATTERN.test(name);
}

/** §4: address = `{applicationCode}.{name}`. Empty until both sides are known. */
export function buildFunctionAddress(
	applicationCode: string | null | undefined,
	name: string | null | undefined,
): string {
	if (!applicationCode || !name) return "";
	return `${applicationCode}.${name}`;
}

// --- describe (§5.4) ---------------------------------------------------

export interface DescribeEndpoint {
	method: string;
	path: string;
	auth: string;
	maxBodyBytes?: number;
}

export interface DescribeSubscription {
	eventType: string;
	path: string;
	mode?: string;
	maxRetries?: number;
	dataOnly?: boolean;
}

export interface DescribeSchedule {
	cron: string;
	timezone?: string;
	path: string;
}

export interface FunctionDescribe {
	abi?: number;
	endpoints: DescribeEndpoint[];
	subscriptions: DescribeSubscription[];
	schedules: DescribeSchedule[];
	config: string[];
	secrets: string[];
	db: string[];
	httpAllow: string[];
	emits: string[];
}

const EMPTY_DESCRIBE: FunctionDescribe = {
	endpoints: [],
	subscriptions: [],
	schedules: [],
	config: [],
	secrets: [],
	db: [],
	httpAllow: [],
	emits: [],
};

function asStringArray(value: unknown): string[] {
	if (!Array.isArray(value)) return [];
	return value.filter((v): v is string => typeof v === "string");
}

/**
 * Parses a version's `describe` JSON (typed `unknown` on the wire) into a
 * shape safe to render. Never throws: a malformed or absent document (a
 * FAILED version may carry none) renders as all-empty rather than crashing
 * the drawer.
 */
export function parseDescribe(raw: unknown): FunctionDescribe {
	if (!raw || typeof raw !== "object") return { ...EMPTY_DESCRIBE };
	const obj = raw as Record<string, unknown>;

	const endpoints: DescribeEndpoint[] = Array.isArray(obj["endpoints"])
		? (obj["endpoints"] as unknown[])
				.filter((e): e is Record<string, unknown> => !!e && typeof e === "object")
				.map((e) => ({
					method: typeof e["method"] === "string" ? e["method"] : "GET",
					path: typeof e["path"] === "string" ? e["path"] : "",
					auth: typeof e["auth"] === "string" ? e["auth"] : "none",
					maxBodyBytes:
						typeof e["maxBodyBytes"] === "number" ? e["maxBodyBytes"] : undefined,
				}))
		: [];

	const subscriptions: DescribeSubscription[] = Array.isArray(obj["subscriptions"])
		? (obj["subscriptions"] as unknown[])
				.filter((e): e is Record<string, unknown> => !!e && typeof e === "object")
				.map((e) => ({
					eventType: typeof e["eventType"] === "string" ? e["eventType"] : "",
					path: typeof e["path"] === "string" ? e["path"] : "",
					mode: typeof e["mode"] === "string" ? e["mode"] : undefined,
					maxRetries: typeof e["maxRetries"] === "number" ? e["maxRetries"] : undefined,
					dataOnly: typeof e["dataOnly"] === "boolean" ? e["dataOnly"] : undefined,
				}))
		: [];

	const schedules: DescribeSchedule[] = Array.isArray(obj["schedules"])
		? (obj["schedules"] as unknown[])
				.filter((e): e is Record<string, unknown> => !!e && typeof e === "object")
				.map((e) => ({
					cron: typeof e["cron"] === "string" ? e["cron"] : "",
					timezone: typeof e["timezone"] === "string" ? e["timezone"] : undefined,
					path: typeof e["path"] === "string" ? e["path"] : "",
				}))
		: [];

	return {
		abi: typeof obj["abi"] === "number" ? obj["abi"] : undefined,
		endpoints,
		subscriptions,
		schedules,
		config: asStringArray(obj["config"]),
		secrets: asStringArray(obj["secrets"]),
		db: asStringArray(obj["db"]),
		httpAllow: asStringArray(obj["httpAllow"]),
		emits: asStringArray(obj["emits"]),
	};
}

/** Best-effort human text for a FAILED version's `failure` (typed `unknown`). */
export function describeFailure(failure: unknown): string | null {
	if (failure === null || failure === undefined) return null;
	if (typeof failure === "string") return failure;
	if (typeof failure === "object") {
		const obj = failure as Record<string, unknown>;
		if (typeof obj["message"] === "string") return obj["message"];
		if (typeof obj["reason"] === "string") return obj["reason"];
		try {
			return JSON.stringify(failure);
		} catch {
			return "Unknown failure";
		}
	}
	return String(failure);
}

// --- versions / aliases --------------------------------------------------

/** The live version's `describe` if one is promoted, else the newest version's. */
export function liveOrNewestVersion(
	versions: FunctionVersion[],
	aliases: FunctionAlias[],
): FunctionVersion | null {
	if (versions.length === 0) return null;
	const live = aliases.find((a) => a.name === "live");
	if (live) {
		const match = versions.find((v) => v.number === live.version);
		if (match) return match;
	}
	return versions.reduce((newest, v) => (v.number > newest.number ? v : newest));
}

/** Alias names currently pointing at a given version number, e.g. for a Versions table row. */
export function aliasesForVersion(aliases: FunctionAlias[], versionNumber: number): string[] {
	return aliases
		.filter((a) => a.version === versionNumber)
		.map((a) => a.name)
		.toSorted();
}

export function versionDigestShort(digest: string): string {
	return digest.length > 12 ? `${digest.slice(0, 12)}…` : digest;
}

// --- settings --------------------------------------------------------------

export interface MissingSettings {
	config: string[];
	secrets: string[];
	db: string[];
}

/**
 * Declared config/secret/db keys (from the live-or-newest version's describe)
 * that have no value on the platform. Promotion is refused while any exist
 * (§4), so the Settings section surfaces the same list the API would refuse
 * against.
 */
export function computeMissingSettings(
	declared: Pick<FunctionDescribe, "config" | "secrets" | "db">,
	settings: FunctionSetting[],
): MissingSettings {
	const haveKind = (kind: string) =>
		new Set(settings.filter((s) => s.kind === kind).map((s) => s.key));
	const haveConfig = haveKind("CONFIG");
	const haveSecret = haveKind("SECRET");
	const haveDb = haveKind("DB");
	return {
		config: declared.config.filter((k) => !haveConfig.has(k)),
		secrets: declared.secrets.filter((k) => !haveSecret.has(k)),
		db: declared.db.filter((k) => !haveDb.has(k)),
	};
}

export function hasMissingSettings(missing: MissingSettings): boolean {
	return missing.config.length > 0 || missing.secrets.length > 0 || missing.db.length > 0;
}

// --- wiring ------------------------------------------------------------

/**
 * Human summary of a promote/alias-removal `wiring` response (§8.5), for the
 * success toast: "pool fn-billing-invoice-pdf · 2 subscriptions created, 1
 * schedule updated". Returns null when there's nothing to report (e.g. a
 * non-`live` alias, which materialises no wiring).
 */
export function describeWiring(wiring: FunctionWiring | null | undefined): string | null {
	if (!wiring) return null;
	const parts: string[] = [];
	if (wiring.dispatchPoolCode) parts.push(`pool ${wiring.dispatchPoolCode}`);

	const counts: Array<[string, string, number | undefined]> = [
		["subscription", "created", wiring.subscriptionsCreated],
		["subscription", "updated", wiring.subscriptionsUpdated],
		["subscription", "deleted", wiring.subscriptionsDeleted],
		["schedule", "created", wiring.schedulesCreated],
		["schedule", "updated", wiring.schedulesUpdated],
		["schedule", "deleted", wiring.schedulesDeleted],
	];
	for (const [noun, action, count] of counts) {
		if (count && count > 0) parts.push(`${count} ${noun}${count === 1 ? "" : "s"} ${action}`);
	}
	return parts.length > 0 ? parts.join(" · ") : null;
}
