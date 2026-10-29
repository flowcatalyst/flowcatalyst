/**
 * Definition export / import (docs/ui/definition-export-import.md).
 *
 * A detail drawer exports a record's safe values as JSON; the matching
 * create drawer reads that JSON back to fill its form. Every reference is by
 * code (or client identifier), never by id, because ids differ per
 * environment. Nothing here talks to the API or to Vue: builders take the
 * loaded records plus whatever codes the caller could resolve, appliers take
 * the form's available choices and return the values to set plus a notice
 * for everything that could not be applied.
 *
 * The JSON shape and key order are a contract shared with the server-rendered
 * UI (internal/ui/pages/testdata/definitions/*.json).
 */

import type { Application } from "@/api/applications";
import type { Connection } from "@/api/connections";
import type { DispatchPool } from "@/api/dispatch-pools";
import type { ServiceAccount } from "@/api/service-accounts";
import type { Subscription } from "@/api/subscriptions";

export type DefinitionKind =
	| "application"
	| "service-account"
	| "connection"
	| "dispatch-pool"
	| "subscription";

export const DEFINITION_VERSION = 1;

// ---------------------------------------------------------------------------
// Serialising
// ---------------------------------------------------------------------------

type Entry = [key: string, value: unknown];

/** An exported definition: the file text, its download name, and what was left out. */
export interface DefinitionExport {
	text: string;
	filename: string;
	/** Why a key is missing, e.g. "Connection not included: you can't read connections." */
	notes: string[];
}

function isSet(value: unknown): boolean {
	if (value === undefined || value === null || value === "") return false;
	if (Array.isArray(value)) return value.length > 0;
	return true;
}

function compact(obj: Record<string, unknown>): Record<string, unknown> {
	return Object.fromEntries(Object.entries(obj).filter(([, v]) => isSet(v)));
}

/**
 * The file text: the envelope with the definition's keys in the order given,
 * as JSON.stringify(..., null, 2) plus a trailing newline. Unset values
 * (undefined, null, "", empty arrays) are omitted; 0 and false are kept.
 */
export function serializeDefinition(
	kind: DefinitionKind,
	entries: Entry[],
): string {
	const definition = compact(
		Object.fromEntries(
			entries.map(([k, v]) => [
				k,
				Array.isArray(v) && v.some((x) => typeof x === "object" && x !== null)
					? v.map((x) => compact(x as Record<string, unknown>))
					: v,
			]),
		),
	);
	const envelope = {
		flowcatalyst: "definition",
		kind,
		version: DEFINITION_VERSION,
		definition,
	};
	return `${JSON.stringify(envelope, null, 2)}\n`;
}

function finishExport(
	kind: DefinitionKind,
	code: string,
	entries: Entry[],
	notes: string[],
): DefinitionExport {
	return {
		text: serializeDefinition(kind, entries),
		filename: `${kind}-${code}.json`,
		notes,
	};
}

function notIncluded(label: string, plural: string): string {
	return `${label} not included: you can't read ${plural}.`;
}

// ---------------------------------------------------------------------------
// Building (export)
// ---------------------------------------------------------------------------

export function buildApplicationDefinition(app: Application): DefinitionExport {
	return finishExport(
		"application",
		app.code,
		[
			["code", app.code],
			["name", app.name],
			["description", app.description],
			["type", app.type],
			["defaultBaseUrl", app.defaultBaseUrl],
			["iconUrl", app.iconUrl],
			["website", app.website],
			["logo", app.logo],
			["logoMimeType", app.logoMimeType],
		],
		[],
	);
}

export interface ClientRef {
	id: string;
	identifier: string;
}

export interface ServiceAccountExportRefs {
	/** Every client the viewer can read; undefined when the list could not be read. */
	clients?: ClientRef[];
	/** The bound application's code; undefined when it could not be read. */
	applicationCode?: string;
}

export function buildServiceAccountDefinition(
	sa: ServiceAccount,
	refs: ServiceAccountExportRefs,
): DefinitionExport {
	const notes: string[] = [];

	let clientIdentifiers: string[] | undefined;
	if (sa.clientIds.length > 0) {
		const found = sa.clientIds.map(
			(id) => refs.clients?.find((c) => c.id === id)?.identifier,
		);
		if (found.every((i): i is string => !!i)) {
			clientIdentifiers = found;
		} else {
			notes.push(notIncluded("Client identifiers", "clients"));
		}
	}

	if (sa.applicationId && !refs.applicationCode) {
		notes.push(notIncluded("Application", "applications"));
	}

	return finishExport(
		"service-account",
		sa.code,
		[
			["code", sa.code],
			["name", sa.name],
			["description", sa.description],
			["scope", sa.scope],
			["clientIdentifiers", clientIdentifiers],
			["applicationCode", refs.applicationCode],
			["roles", sa.roles],
		],
		notes,
	);
}

export function buildConnectionDefinition(
	conn: Connection,
	refs: { serviceAccountCode?: string },
): DefinitionExport {
	const notes: string[] = [];
	if (!refs.serviceAccountCode) {
		notes.push(notIncluded("Service account", "service accounts"));
	}
	if (conn.clientId && !conn.clientIdentifier) {
		notes.push(notIncluded("Client", "clients"));
	}
	return finishExport(
		"connection",
		conn.code,
		[
			["code", conn.code],
			["name", conn.name],
			["description", conn.description],
			["externalId", conn.externalId],
			["serviceAccountCode", refs.serviceAccountCode],
			["clientIdentifier", conn.clientIdentifier],
		],
		notes,
	);
}

export function buildDispatchPoolDefinition(pool: DispatchPool): DefinitionExport {
	const notes: string[] = [];
	if (pool.clientId && !pool.clientIdentifier) {
		notes.push(notIncluded("Client", "clients"));
	}
	return finishExport(
		"dispatch-pool",
		pool.code,
		[
			["code", pool.code],
			["name", pool.name],
			["description", pool.description],
			["rateLimit", pool.rateLimit],
			["concurrency", pool.concurrency],
			["clientIdentifier", pool.clientIdentifier],
		],
		notes,
	);
}

export function buildSubscriptionDefinition(
	sub: Subscription,
	refs: { connectionCode?: string },
): DefinitionExport {
	const notes: string[] = [];
	if (sub.connectionId && !refs.connectionCode) {
		notes.push(notIncluded("Connection", "connections"));
	}
	if (sub.dispatchPoolId && !sub.dispatchPoolCode) {
		notes.push(notIncluded("Dispatch pool", "dispatch pools"));
	}
	if (sub.clientId && !sub.clientIdentifier) {
		notes.push(notIncluded("Client", "clients"));
	}
	return finishExport(
		"subscription",
		sub.code,
		[
			["code", sub.code],
			["applicationCode", sub.applicationCode],
			["name", sub.name],
			["description", sub.description],
			["endpoint", sub.endpoint],
			["clientScoped", sub.clientScoped],
			["clientIdentifier", sub.clientIdentifier],
			[
				"eventTypes",
				(sub.eventTypes ?? []).map((et) => ({
					eventTypeCode: et.eventTypeCode,
					specVersion: et.specVersion,
				})),
			],
			["connectionCode", refs.connectionCode],
			["queue", sub.queue],
			["dispatchPoolCode", sub.dispatchPoolCode],
			["mode", sub.mode],
			["maxAgeSeconds", sub.maxAgeSeconds],
			["timeoutSeconds", sub.timeoutSeconds],
			["delaySeconds", sub.delaySeconds],
			["sequence", sub.sequence],
		],
		notes,
	);
}

// ---------------------------------------------------------------------------
// Parsing (import)
// ---------------------------------------------------------------------------

export type ParsedDefinition =
	| { ok: true; definition: Record<string, unknown> }
	| { ok: false; error: string };

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Checks the envelope; on any mismatch the form is left untouched. */
export function parseDefinition(
	text: string,
	kind: DefinitionKind,
): ParsedDefinition {
	let doc: unknown;
	try {
		doc = JSON.parse(text);
	} catch {
		return { ok: false, error: "This is not valid JSON." };
	}
	if (!isRecord(doc) || doc["flowcatalyst"] !== "definition") {
		return {
			ok: false,
			error: 'This is not a FlowCatalyst definition (expected "flowcatalyst": "definition").',
		};
	}
	if (doc["kind"] !== kind) {
		const got = typeof doc["kind"] === "string" ? doc["kind"] : "unknown";
		return {
			ok: false,
			error: `This is a ${got} definition; this form takes a ${kind} definition.`,
		};
	}
	if (doc["version"] !== DEFINITION_VERSION) {
		return {
			ok: false,
			error: `Unsupported definition version ${JSON.stringify(doc["version"])}; this form reads version ${DEFINITION_VERSION}.`,
		};
	}
	if (!isRecord(doc["definition"])) {
		return { ok: false, error: "The definition is missing." };
	}
	return { ok: true, definition: doc["definition"] };
}

/** The client identifiers a definition names, so the caller can look them up before applying. */
export function definitionClientIdentifiers(
	definition: Record<string, unknown>,
): string[] {
	const out: string[] = [];
	const one = definition["clientIdentifier"];
	if (typeof one === "string" && one) out.push(one);
	const many = definition["clientIdentifiers"];
	if (Array.isArray(many)) {
		for (const v of many) if (typeof v === "string" && v) out.push(v);
	}
	return out;
}

// ---------------------------------------------------------------------------
// Applying (import)
// ---------------------------------------------------------------------------

/** What an import control reports back to its create form. */
export interface ImportOutcome {
	/** Set when the text was refused; the form is untouched. */
	error?: string;
	notices?: string[];
}

export interface ApplyResult<V> {
	/** Only the fields the definition supplied and the form can take. */
	values: V;
	/** Unresolved references, values the form can't take, ignored keys. */
	notices: string[];
}

/** A record the form offers, matched by code. */
export interface CodeChoice {
	id: string;
	code: string;
}

export interface EventTypeChoice extends CodeChoice {
	clientScoped: boolean;
	/** The CURRENT spec version, which is what the create form binds. */
	currentVersion: string;
}

/** Reads a definition's keys, noting wrong-typed values and, at the end, unknown keys. */
class Reader {
	readonly notices: string[] = [];
	private readonly known = new Set<string>();

	private readonly def: Record<string, unknown>;

	constructor(def: Record<string, unknown>) {
		this.def = def;
	}

	/** The key's raw value, marking the key as understood. */
	raw(key: string): unknown {
		this.known.add(key);
		return this.def[key];
	}

	private wrong(key: string, value: unknown, want: string) {
		this.notices.push(
			`Not applied by this form: ${key} should be ${want} (got ${JSON.stringify(value)}).`,
		);
	}

	string(key: string): string | undefined {
		const v = this.raw(key);
		if (v === undefined || v === null) return undefined;
		if (typeof v !== "string") {
			this.wrong(key, v, "text");
			return undefined;
		}
		return v;
	}

	number(key: string): number | undefined {
		const v = this.raw(key);
		if (v === undefined || v === null) return undefined;
		if (typeof v !== "number" || !Number.isFinite(v)) {
			this.wrong(key, v, "a number");
			return undefined;
		}
		return v;
	}

	boolean(key: string): boolean | undefined {
		const v = this.raw(key);
		if (v === undefined || v === null) return undefined;
		if (typeof v !== "boolean") {
			this.wrong(key, v, "true or false");
			return undefined;
		}
		return v;
	}

	strings(key: string): string[] | undefined {
		const v = this.raw(key);
		if (v === undefined || v === null) return undefined;
		if (!Array.isArray(v) || !v.every((x) => typeof x === "string")) {
			this.wrong(key, v, "a list of text values");
			return undefined;
		}
		return v as string[];
	}

	/** One of `allowed`, else a notice and undefined. */
	oneOf<T extends string>(key: string, allowed: readonly T[]): T | undefined {
		const v = this.string(key);
		if (v === undefined) return undefined;
		if ((allowed as readonly string[]).includes(v)) return v as T;
		this.notices.push(
			`Not applied by this form: ${key} "${v}" is not one of ${allowed.join(", ")}.`,
		);
		return undefined;
	}

	/** A key the form has no field for: listed with its value. */
	notApplied(key: string, hint?: string) {
		const v = this.raw(key);
		if (v === undefined || v === null) return;
		const shown = Array.isArray(v) ? v.join(", ") : JSON.stringify(v);
		if (hint) {
			this.notices.push(`${hint}: ${shown}`);
		} else {
			this.notices.push(`Not applied by this form: ${key} ${shown}.`);
		}
	}

	unresolved(label: string, code: string, extra = "") {
		this.notices.push(
			`${label} "${code}" was not found here${extra}. Import or create it first, then choose it.`,
		);
	}

	finish(): string[] {
		const unknown = Object.keys(this.def).filter((k) => !this.known.has(k));
		if (unknown.length > 0) {
			this.notices.push(`Ignored (unknown keys): ${unknown.join(", ")}.`);
		}
		return this.notices;
	}

	/** Matches a code against choices; a miss is noted as unresolved. */
	match<C extends { id: string }>(
		key: string,
		label: string,
		choices: C[],
		codeOf: (c: C) => string,
		extra = "",
	): string | undefined {
		const code = this.string(key);
		if (code === undefined) return undefined;
		const hit = choices.find((c) => codeOf(c) === code);
		if (!hit) {
			this.unresolved(label, code, extra);
			return undefined;
		}
		return hit.id;
	}

	/** Like match, for one client identifier. */
	client(key: string, clients: ClientRef[]): string | undefined {
		return this.match(
			key,
			"Client",
			clients,
			(c) => c.identifier,
			" (or you can't access it)",
		);
	}
}

const APPLICATION_TYPES = ["APPLICATION", "INTEGRATION"] as const;
const SCOPES = ["ANCHOR", "PARTNER", "CLIENT"] as const;
const MODES = ["IMMEDIATE", "NEXT_ON_ERROR", "BLOCK_ON_ERROR"] as const;
const QUEUES = ["DEFAULT", "HIGH_PRIORITY"] as const;

export interface ApplicationFormValues {
	type?: (typeof APPLICATION_TYPES)[number];
	code?: string;
	name?: string;
	description?: string;
	defaultBaseUrl?: string;
	iconUrl?: string;
	website?: string;
	logo?: string;
	logoMimeType?: string;
}

export function applyApplicationDefinition(
	def: Record<string, unknown>,
): ApplyResult<ApplicationFormValues> {
	const r = new Reader(def);
	const values: ApplicationFormValues = {
		code: r.string("code"),
		name: r.string("name"),
		description: r.string("description"),
		type: r.oneOf("type", APPLICATION_TYPES),
		defaultBaseUrl: r.string("defaultBaseUrl"),
		iconUrl: r.string("iconUrl"),
		website: r.string("website"),
		logo: r.string("logo"),
		logoMimeType: r.string("logoMimeType"),
	};
	return { values: defined(values), notices: r.finish() };
}

export interface ServiceAccountFormValues {
	code?: string;
	name?: string;
	description?: string;
	scope?: (typeof SCOPES)[number];
	selectedClientIds?: string[];
}

export function applyServiceAccountDefinition(
	def: Record<string, unknown>,
	choices: { clients: ClientRef[] },
): ApplyResult<ServiceAccountFormValues> {
	const r = new Reader(def);
	const values: ServiceAccountFormValues = {
		code: r.string("code"),
		name: r.string("name"),
		description: r.string("description"),
		scope: r.oneOf("scope", SCOPES),
	};

	const identifiers = r.strings("clientIdentifiers");
	if (identifiers) {
		const ids: string[] = [];
		for (const identifier of identifiers) {
			const hit = choices.clients.find((c) => c.identifier === identifier);
			if (hit) ids.push(hit.id);
			else r.unresolved("Client", identifier, " (or you can't access it)");
		}
		if (ids.length > 0) values.selectedClientIds = ids;
	}

	r.notApplied("applicationCode");
	r.notApplied("roles", "Assign roles after creating");
	return { values: defined(values), notices: r.finish() };
}

export interface ConnectionFormValues {
	code?: string;
	name?: string;
	description?: string;
	externalId?: string;
	serviceAccountId?: string;
	clientId?: string;
}

export function applyConnectionDefinition(
	def: Record<string, unknown>,
	choices: { serviceAccounts: CodeChoice[]; clients: ClientRef[] },
): ApplyResult<ConnectionFormValues> {
	const r = new Reader(def);
	const values: ConnectionFormValues = {
		code: r.string("code"),
		name: r.string("name"),
		description: r.string("description"),
		externalId: r.string("externalId"),
		serviceAccountId: r.match(
			"serviceAccountCode",
			"Service account",
			choices.serviceAccounts,
			(c) => c.code,
		),
		clientId: r.client("clientIdentifier", choices.clients),
	};
	return { values: defined(values), notices: r.finish() };
}

export interface DispatchPoolFormValues {
	code?: string;
	name?: string;
	description?: string;
	rateLimit?: number;
	concurrency?: number;
	clientId?: string;
}

export function applyDispatchPoolDefinition(
	def: Record<string, unknown>,
	choices: { clients: ClientRef[] },
): ApplyResult<DispatchPoolFormValues> {
	const r = new Reader(def);
	const values: DispatchPoolFormValues = {
		code: r.string("code"),
		name: r.string("name"),
		description: r.string("description"),
		rateLimit: r.number("rateLimit"),
		concurrency: r.number("concurrency"),
		clientId: r.client("clientIdentifier", choices.clients),
	};
	return { values: defined(values), notices: r.finish() };
}

export interface SubscriptionFormValues {
	code?: string;
	name?: string;
	description?: string;
	endpoint?: string;
	clientScoped?: boolean;
	clientId?: string;
	/** Ids of the event types to select. */
	selectedEventTypes?: string[];
	connectionId?: string;
	queue?: (typeof QUEUES)[number];
	dispatchPoolId?: string;
	mode?: (typeof MODES)[number];
	maxAgeSeconds?: number;
	timeoutSeconds?: number;
	delaySeconds?: number;
	sequence?: number;
}

export function applySubscriptionDefinition(
	def: Record<string, unknown>,
	choices: {
		connections: CodeChoice[];
		dispatchPools: CodeChoice[];
		eventTypes: EventTypeChoice[];
		clients: ClientRef[];
	},
): ApplyResult<SubscriptionFormValues> {
	const r = new Reader(def);
	const clientScoped = r.boolean("clientScoped");
	const values: SubscriptionFormValues = {
		code: r.string("code"),
		name: r.string("name"),
		description: r.string("description"),
		endpoint: r.string("endpoint"),
		clientScoped,
		connectionId: r.match(
			"connectionCode",
			"Connection",
			choices.connections,
			(c) => c.code,
		),
		queue: r.oneOf("queue", QUEUES),
		dispatchPoolId: r.match(
			"dispatchPoolCode",
			"Dispatch pool",
			choices.dispatchPools,
			(c) => c.code,
		),
		mode: r.oneOf("mode", MODES),
		maxAgeSeconds: r.number("maxAgeSeconds"),
		timeoutSeconds: r.number("timeoutSeconds"),
		delaySeconds: r.number("delaySeconds"),
		sequence: r.number("sequence"),
	};

	// The form shows the client field only for a client-scoped subscription.
	const clientId = r.client("clientIdentifier", choices.clients);
	if (clientId !== undefined) {
		if (clientScoped) {
			values.clientId = clientId;
		} else {
			r.notApplied(
				"clientIdentifier",
				"Not applied by this form (the subscription is not client scoped)",
			);
		}
	}

	// The form offers only event types whose client scoping matches.
	const bindings = readEventTypeBindings(r);
	const ids: string[] = [];
	for (const b of bindings) {
		const et = choices.eventTypes.find((e) => e.code === b.eventTypeCode);
		if (!et) {
			r.unresolved("Event type", b.eventTypeCode);
		} else if (et.clientScoped !== !!clientScoped) {
			r.notices.push(
				`Event type "${b.eventTypeCode}" is ${et.clientScoped ? "" : "not "}client scoped, which does not match this subscription's client scoping, so it was not selected.`,
			);
		} else {
			ids.push(et.id);
			if (b.specVersion && b.specVersion !== et.currentVersion) {
				r.notices.push(
					`Event type "${b.eventTypeCode}": version ${b.specVersion} is not the current version (${et.currentVersion}); the form binds the current version.`,
				);
			}
		}
	}
	if (ids.length > 0) values.selectedEventTypes = ids;

	r.notApplied("applicationCode");
	return { values: defined(values), notices: r.finish() };
}

function readEventTypeBindings(
	r: Reader,
): { eventTypeCode: string; specVersion?: string }[] {
	const raw = r.raw("eventTypes");
	if (raw === undefined || raw === null) return [];
	if (!Array.isArray(raw)) {
		r.notApplied("eventTypes");
		return [];
	}
	const out: { eventTypeCode: string; specVersion?: string }[] = [];
	for (const item of raw) {
		if (isRecord(item) && typeof item["eventTypeCode"] === "string") {
			const v = item["specVersion"];
			out.push({
				eventTypeCode: item["eventTypeCode"],
				...(typeof v === "string" && v ? { specVersion: v } : {}),
			});
		} else {
			r.notices.push(
				`Not applied by this form: an eventTypes entry without an eventTypeCode (${JSON.stringify(item)}).`,
			);
		}
	}
	return out;
}

/** Drops undefined entries so callers can test `!== undefined` per field. */
function defined<T extends object>(values: T): T {
	return Object.fromEntries(
		Object.entries(values).filter(([, v]) => v !== undefined),
	) as T;
}
