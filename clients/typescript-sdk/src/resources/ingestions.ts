/**
 * Ingestions Resource — definitions, versions, test connection and sets.
 *
 * Hand-typed (does not depend on generated OpenAPI types), like
 * `ScheduledJobsResource`. The runtime operations
 * (`/api/ingestion-runtime/v1/*`) are the runtime's own client and are
 * deliberately not here.
 *
 * Secrets in a definition are NAMES (a secret reference the runtime
 * resolves); nothing here ever carries a secret value.
 */

import type { ResultAsync } from "neverthrow";
import type { SdkError } from "../errors.js";
import type { FlowCatalystClient } from "../client.js";

// ── Definition types ────────────────────────────────────────────────────────

export type IngestionStatus = "ACTIVE" | "PAUSED";
export type TargetMode = "IMMEDIATE" | "NEXT_ON_ERROR" | "BLOCK_ON_ERROR";

/** Source kinds the platform ships; a runtime may report more. */
export type BuiltInSourceKind =
	| "http"
	| "sftp"
	| "s3"
	| "sqs"
	| "rabbitmq"
	| "database-poll"
	| "api-poll";

/** A built-in source: `kind` plus that kind's settings (names for secrets). */
export interface BuiltInIngestionSource {
	kind: BuiltInSourceKind;
	[setting: string]: unknown;
}

/**
 * Escape hatch for a kind a runtime extension reports (`custom`, or e.g.
 * `camel-route`): the kind carries its own `settings`.
 */
export interface CustomIngestionSource {
	kind: "custom" | (string & {});
	settings?: Record<string, unknown>;
}

export type IngestionSource = BuiltInIngestionSource | CustomIngestionSource;

/** How a payload becomes records. */
export type IngestionSplit =
	| { kind: "none" }
	| { kind: "csv"; header?: boolean }
	| { kind: "json"; arrayPath?: string }
	| { kind: "xml"; element?: string };

/** The dispatch job each record becomes. `connection` and `serviceAccount` are exclusive. */
export interface IngestionTarget {
	code: string;
	targetUrl: string;
	/** Dispatch pool code. */
	pool: string;
	/** Connection code; its service account signs the deliveries. */
	connection?: string;
	/** Service account code, for signing without a connection. */
	serviceAccount?: string;
	mode: TargetMode;
	batchSize: number;
}

/** The part of an ingestion that is versioned. */
export interface IngestionDefinition {
	name: string;
	source: IngestionSource;
	split: IngestionSplit;
	target: IngestionTarget;
	keepFailedPayloads?: boolean;
}

// ── Responses ───────────────────────────────────────────────────────────────

export interface Ingestion extends IngestionDefinition {
	id: string;
	applicationId: string;
	applicationCode: string;
	clientId: string;
	clientIdentifier: string;
	code: string;
	status: IngestionStatus;
	version: number;
	latestVersion: number;
	/** Id of the set that owns the ingestion; read-only until the set is released. */
	managedBy?: string;
	runtimeNotice?: string;
	createdAt: string;
	updatedAt: string;
}

export interface IngestionList {
	ingestions: Ingestion[];
	hasMore: boolean;
	/** Pass as `after` to read the next page; absent on the last. */
	nextCursor?: string;
}

export interface IngestionVersion extends IngestionDefinition {
	id: string;
	version: number;
	live: boolean;
	note?: string;
	appliedFrom?: string;
	createdAt: string;
	createdBy?: string;
}

export interface IngestionHistoryEntry {
	id: string;
	operation: string;
	performedAt: string;
	principalId?: string;
	principalName?: string;
	details?: unknown;
}

export interface IngestionHistory {
	entries: IngestionHistoryEntry[];
	hasMore: boolean;
}

export interface IngestionFailure {
	id: string;
	kind: string;
	message: string;
	afterRecords: number;
	occurredAt: string;
	runtimeId?: string;
	version?: number;
	/** Only for an ingestion that keeps failed payloads. */
	excerpt?: string;
}

export interface IngestionFailureList {
	failures: IngestionFailure[];
	hasMore: boolean;
	nextCursor?: string;
}

export interface IngestionSourceKind {
	kind: string;
	builtIn: boolean;
	singleOwner: boolean;
	description?: string;
	/** Creating or changing an ingestion of this kind needs this permission. */
	requiredPermission?: string;
	settings?: unknown;
}

export type IngestionTestStatus = "PENDING" | "RUNNING" | "DONE" | "EXPIRED";
export type IngestionTestOutcome =
	| "RESULTS"
	| "NO_RESULTS"
	| "REFUSED"
	| "FAILED"
	| "NOT_SUPPORTED";

export interface IngestionTest {
	id: string;
	ingestionId?: string;
	kind: string;
	status: IngestionTestStatus;
	outcome?: IngestionTestOutcome;
	records?: number;
	steps?: string[];
	excerpt?: string;
	/** Set while no live runtime carries the kind: the test waits, nothing has failed. */
	notice?: string;
	runtimeId?: string;
	createdAt: string;
	completedAt?: string;
	expiresAt: string;
}

// ── Sets ────────────────────────────────────────────────────────────────────

export type SetChangeAction = "CREATE" | "CHANGE" | "ADOPT" | "UNCHANGED" | "REMOVE";

export interface SetChange {
	application: string;
	client: string;
	code: string;
	action: SetChangeAction;
	changed: string[];
	ingestionId?: string;
}

export interface SetFile {
	path: string;
	content: string;
}

/** One ingestion of a set. `active` defaults to true; an apply sets it. */
export interface IngestionSetItem extends IngestionDefinition {
	application: string;
	client: string;
	code: string;
	active?: boolean;
}

export interface ApplyIngestionSetRequest {
	items: IngestionSetItem[];
	/** The files the items were read from, kept with the apply (at most 2 MB in all). */
	files?: SetFile[];
}

export interface IngestionSet {
	id: string;
	name: string;
	ingestions: number;
	createdAt: string;
	updatedAt: string;
}

export interface IngestionSetPlan {
	name: string;
	dryRun: boolean;
	/** The recorded apply; absent for a dry run and for an apply that changed nothing. */
	applyId?: string;
	changes: SetChange[];
}

export interface IngestionSetApply {
	id: string;
	set: string;
	appliedAt: string;
	appliedBy?: string;
	items: number;
	files: number;
	changes: SetChange[];
	goingBackTo?: string;
}

/** An apply with the items and files it was given. */
export interface IngestionSetApplyArchive {
	set: string;
	applyId: string;
	appliedAt: string;
	appliedBy: string;
	items: IngestionSetItem[];
	files: SetFile[];
	goingBackTo?: string;
}

export interface IngestionSetRelease {
	name: string;
	released: number;
}

// ── Requests ────────────────────────────────────────────────────────────────

export interface CreateIngestionRequest extends IngestionDefinition {
	/** Code of the owning application, which must be of type INTEGRATION. */
	application: string;
	/** The client's id (clt_...) or identifier. */
	client: string;
	/** Lowercase letters, digits and hyphens, unique within the application and client. */
	code: string;
	note?: string;
	/** Make the first version live. Default true. */
	live?: boolean;
}

export interface UpdateIngestionRequest extends IngestionDefinition {
	/** The live version the caller read (0 when none is live); a different current version is a 409. */
	expectedVersion: number;
	note?: string;
}

export interface SaveIngestionVersionRequest extends IngestionDefinition {
	note?: string;
	/** Also make the saved version live (needs the promote permission). Default false. */
	live?: boolean;
}

export interface ListIngestionsFilters {
	applicationCode?: string;
	/** Client id or identifier. */
	clientId?: string;
	q?: string;
	status?: IngestionStatus;
	kind?: string;
	after?: string;
	limit?: number;
}

export interface CursorPage {
	after?: string;
	limit?: number;
}

export interface HistoryPage {
	limit?: number;
	offset?: number;
}

/** Test a saved ingestion's live definition, or a definition without saving it. */
export type TestIngestionRequest =
	| { ingestionId: string }
	| {
			definition: IngestionDefinition;
			application: string;
			client: string;
	  };

// ── Resource ────────────────────────────────────────────────────────────────

const PATH = "/api/ingestions";
const SETS_PATH = "/api/ingestion-sets";

const enc = encodeURIComponent;

export class IngestionsResource {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	list(filters: ListIngestionsFilters = {}): ResultAsync<IngestionList, SdkError> {
		return this.json("GET", `${PATH}${qs({ ...filters })}`);
	}

	create(req: CreateIngestionRequest): ResultAsync<Ingestion, SdkError> {
		return this.json("POST", PATH, req);
	}

	get(id: string): ResultAsync<Ingestion, SdkError> {
		return this.json("GET", `${PATH}/${enc(id)}`);
	}

	update(id: string, req: UpdateIngestionRequest): ResultAsync<Ingestion, SdkError> {
		return this.json("PUT", `${PATH}/${enc(id)}`, req);
	}

	delete(id: string): ResultAsync<void, SdkError> {
		return this.json("DELETE", `${PATH}/${enc(id)}`);
	}

	pause(id: string): ResultAsync<Ingestion, SdkError> {
		return this.json("POST", `${PATH}/${enc(id)}/pause`);
	}

	resume(id: string): ResultAsync<Ingestion, SdkError> {
		return this.json("POST", `${PATH}/${enc(id)}/resume`);
	}

	listVersions(id: string): ResultAsync<{ versions: IngestionVersion[] }, SdkError> {
		return this.json("GET", `${PATH}/${enc(id)}/versions`);
	}

	saveVersion(
		id: string,
		req: SaveIngestionVersionRequest,
	): ResultAsync<IngestionVersion, SdkError> {
		return this.json("POST", `${PATH}/${enc(id)}/versions`, req);
	}

	makeVersionLive(id: string, version: number): ResultAsync<Ingestion, SdkError> {
		return this.json("POST", `${PATH}/${enc(id)}/versions/${enc(String(version))}/live`);
	}

	/** The definition file `fcdev ingest apply` reads back, as YAML text. */
	getDefinition(id: string): ResultAsync<string, SdkError> {
		return this.request<string>("GET", `${PATH}/${enc(id)}/definition`, undefined, true);
	}

	listHistory(
		id: string,
		page: HistoryPage = {},
	): ResultAsync<IngestionHistory, SdkError> {
		return this.json("GET", `${PATH}/${enc(id)}/history${qs({ ...page })}`);
	}

	listFailures(
		id: string,
		page: CursorPage = {},
	): ResultAsync<IngestionFailureList, SdkError> {
		return this.json("GET", `${PATH}/${enc(id)}/failures${qs({ ...page })}`);
	}

	listSourceKinds(): ResultAsync<{ kinds: IngestionSourceKind[] }, SdkError> {
		return this.json("GET", "/api/ingestion-source-kinds");
	}

	/** Start a pull-based connection test (202); poll it with `getTest`. */
	testConnection(req: TestIngestionRequest): ResultAsync<IngestionTest, SdkError> {
		return this.json("POST", "/api/ingestion-tests", req);
	}

	getTest(id: string): ResultAsync<IngestionTest, SdkError> {
		return this.json("GET", `/api/ingestion-tests/${enc(id)}`);
	}

	// ── Sets ──

	listSets(): ResultAsync<{ sets: IngestionSet[] }, SdkError> {
		return this.json("GET", SETS_PATH);
	}

	/** Apply a set; `dryRun` reports the changes and writes nothing. */
	applySet(
		name: string,
		req: ApplyIngestionSetRequest,
		dryRun?: boolean,
	): ResultAsync<IngestionSetPlan, SdkError> {
		return this.json("PUT", `${SETS_PATH}/${enc(name)}${qs({ dryRun })}`, req);
	}

	listSetApplies(name: string): ResultAsync<{ applies: IngestionSetApply[] }, SdkError> {
		return this.json("GET", `${SETS_PATH}/${enc(name)}/applies`);
	}

	getSetApply(
		name: string,
		applyId: string,
	): ResultAsync<IngestionSetApplyArchive, SdkError> {
		return this.json("GET", `${SETS_PATH}/${enc(name)}/applies/${enc(applyId)}`);
	}

	/** Apply an archived apply's items again as a new apply. */
	reapplySet(
		name: string,
		applyId: string,
		dryRun?: boolean,
	): ResultAsync<IngestionSetPlan, SdkError> {
		return this.json(
			"POST",
			`${SETS_PATH}/${enc(name)}/applies/${enc(applyId)}/reapply${qs({ dryRun })}`,
		);
	}

	/** End the set's ownership; changes nothing else. */
	releaseSet(name: string): ResultAsync<IngestionSetRelease, SdkError> {
		return this.json("POST", `${SETS_PATH}/${enc(name)}/release`);
	}

	// ── Internal: thin fetch wrapper that reuses the client's auth + retry. ──

	private json<T>(method: string, path: string, body?: unknown): ResultAsync<T, SdkError> {
		return this.request<T>(method, path, body, false);
	}

	private request<T>(
		method: string,
		path: string,
		body: unknown,
		text: boolean,
	): ResultAsync<T, SdkError> {
		return this.client.request<T>(async (_genClient, headers) => {
			const url =
				(this.client as unknown as { config: { baseUrl: string } }).config.baseUrl + path;
			const init: RequestInit = {
				method,
				headers: {
					...headers,
					...(body !== undefined ? { "Content-Type": "application/json" } : {}),
				},
				body: body !== undefined ? JSON.stringify(body) : undefined,
			};
			const response = await fetch(url, init);
			if (!response.ok) {
				let errorBody: unknown = undefined;
				try {
					errorBody = await response.json();
				} catch {
					/* response had no JSON body */
				}
				return { error: errorBody, response };
			}
			if (response.status === 204) {
				return { data: undefined as unknown as T, response };
			}
			const data = text ? await response.text() : await response.json();
			return { data, response };
		});
	}
}

/** Build a `?key=value&...` querystring from a flat object. Skips nullish. */
function qs(obj: Record<string, unknown>): string {
	const entries = Object.entries(obj).filter(
		([, v]) => v !== undefined && v !== null && v !== "",
	);
	if (entries.length === 0) return "";
	const sp = new URLSearchParams();
	for (const [k, v] of entries) sp.append(k, String(v));
	return `?${sp.toString()}`;
}
