/**
 * Definition types for syncing FlowCatalyst primitives to the platform.
 *
 * These types describe the shape of things an application declares about
 * itself: the roles it needs, the event types it publishes, the
 * subscriptions it consumes, the dispatch pools it expects, and the
 * principals it manages.
 *
 * You build a `DefinitionSet` (one per application) and pass it to
 * `client.definitions().sync(...)`. See `docs/syncing-definitions.md` for
 * a full walkthrough.
 */

// ────────────────────────────────────────────────────────────────────────────
// Role
// ────────────────────────────────────────────────────────────────────────────

/**
 * A role declaration.
 *
 * Names are stored with the application code prefix: given `name: "admin"`
 * under application `orders`, the role is persisted as `orders:admin`. Do
 * not include the prefix in `name` yourself — the platform adds it.
 *
 * Permissions follow the 4-part format `<domain>:<area>:<resource>:<action>`
 * (e.g. `orders:admin:shipment:cancel`). Wildcards are supported in any
 * position. See `docs/syncing-definitions.md` for the permission conventions.
 */
export interface RoleDefinition {
	/** Short name (no `<app>:` prefix — the platform adds it) */
	name: string;
	/** Human-readable label */
	displayName?: string;
	description?: string;
	/** Permission strings (4-part format) */
	permissions?: string[];
	/**
	 * When true, client admins can assign this role to their own users.
	 * When false, only platform admins can assign it.
	 */
	clientManaged?: boolean;
}

// ────────────────────────────────────────────────────────────────────────────
// Event type
// ────────────────────────────────────────────────────────────────────────────

/**
 * An event type declaration.
 *
 * `code` is the full 4-part identifier `<app>:<subdomain>:<aggregate>:<event>`.
 * The first segment MUST match the application code being synced.
 *
 * JSON Schema for the event payload is not sync'd via this endpoint — attach
 * schemas through the admin UI or the per-resource `eventTypes.addSchema(...)`
 * API. See `docs/syncing-definitions.md#event-types`.
 */
export interface EventTypeDefinition {
	/** Full code: `<app>:<subdomain>:<aggregate>:<event>` */
	code: string;
	/** Human-readable label */
	name: string;
	description?: string;
}

// ────────────────────────────────────────────────────────────────────────────
// Subscription
// ────────────────────────────────────────────────────────────────────────────

/** How dispatch job failures interact with this subscription's delivery order. */
export type SubscriptionMode =
	| "IMMEDIATE" // deliver independently; failures don't block other deliveries
	| "BLOCK_ON_ERROR"; // on failure, hold subsequent deliveries for the same message group

/** A single event-type binding inside a subscription. */
export interface SubscriptionEventTypeBinding {
	/** Full event type code (must exist at sync time) */
	eventTypeCode: string;
	/** Optional filter expression (matches platform conventions) */
	filter?: string;
}

/**
 * A subscription declaration.
 *
 * The subscription describes a downstream consumer: where to deliver
 * (`target` URL or `connectionId` reference), which event types trigger it,
 * and how to handle failures.
 */
export interface SubscriptionDefinition {
	/** Short code (unique within the application) */
	code: string;
	name: string;
	description?: string;
	/** Webhook URL where events are delivered */
	target: string;
	/** Pre-configured connection reference (alternative to `target`) */
	connectionId?: string;
	/** Event types this subscription consumes */
	eventTypes: SubscriptionEventTypeBinding[];
	/** Dispatch pool code; falls back to the platform default when omitted */
	dispatchPoolCode?: string;
	/** Delivery mode; default IMMEDIATE */
	mode?: SubscriptionMode;
	maxRetries?: number;
	timeoutSeconds?: number;
	/** When true, only the event's `data` field is POSTed (no metadata envelope) */
	dataOnly?: boolean;
}

// ────────────────────────────────────────────────────────────────────────────
// Dispatch pool
// ────────────────────────────────────────────────────────────────────────────

/**
 * A dispatch pool declaration.
 *
 * Pools control how the platform schedules outbound delivery — concurrency
 * cap and per-minute rate limit. Segregate workloads with different
 * latency profiles into different pools (see the adaptive-concurrency
 * design for why).
 */
export interface DispatchPoolDefinition {
	code: string;
	name: string;
	description?: string;
	/** Rate limit in requests per minute; default 100 */
	rateLimit?: number;
	/** Concurrency cap; default 10 */
	concurrency?: number;
}

// ────────────────────────────────────────────────────────────────────────────
// Principal
// ────────────────────────────────────────────────────────────────────────────

/**
 * A principal (user) declaration.
 *
 * Matched by email. `roles` lists role short names WITHOUT the application
 * prefix (the platform adds `<app>:` per role). Principals sync'd this way
 * use the internal identity provider; principals provisioned via OIDC are
 * managed by their IdP's sync flow instead.
 */
export interface PrincipalDefinition {
	email: string;
	name: string;
	/** Role short names (no `<app>:` prefix) */
	roles?: string[];
	/** Default true */
	active?: boolean;
}

// ────────────────────────────────────────────────────────────────────────────
// Process (workflow documentation)
// ────────────────────────────────────────────────────────────────────────────

/**
 * A process documentation declaration.
 *
 * `code` is the full three-segment identifier `<app>:<subdomain>:<process>`,
 * matching event-type conventions. `body` carries the diagram source
 * verbatim — typically Mermaid; override `diagramType` if you publish a
 * different format.
 */
export interface ProcessDefinition {
	code: string;
	name: string;
	description?: string;
	/** Diagram body. Stored verbatim. */
	body?: string;
	/** Diagram language. Platform applies `mermaid` when omitted. */
	diagramType?: string;
	tags?: string[];
}

// ────────────────────────────────────────────────────────────────────────────
// Scheduled job
// ────────────────────────────────────────────────────────────────────────────

/**
 * A scheduled-job declaration.
 *
 * `crons` accepts standard 5-field cron expressions; the platform's
 * scheduler evaluates them in `timezone` (defaults to UTC server-side).
 *
 * `concurrent: true` lets the platform fire a new tick while a previous
 * invocation is still running — most apps want false. Use the SDK's
 * `LockProvider` for in-app dedupe if you need single-fire semantics
 * across pods.
 *
 * `tracksCompletion: true` flips the platform from "webhook delivery is
 * the success signal" to "consumer POSTs back to
 * /api/scheduled-jobs/instances/{id}/complete when done", enabling
 * per-instance status tracking.
 */
export interface ScheduledJobDefinition {
	code: string;
	name: string;
	description?: string;
	crons: string[];
	timezone?: string;
	payload?: unknown;
	concurrent?: boolean;
	tracksCompletion?: boolean;
	timeoutSeconds?: number;
	deliveryMaxAttempts?: number;
	/** Override the application's default callback URL for this job. */
	targetUrl?: string;
}

// ────────────────────────────────────────────────────────────────────────────
// Definition set
// ────────────────────────────────────────────────────────────────────────────

/** Container for all definitions belonging to one application. */
export interface DefinitionSet {
	applicationCode: string;
	roles?: RoleDefinition[];
	eventTypes?: EventTypeDefinition[];
	subscriptions?: SubscriptionDefinition[];
	dispatchPools?: DispatchPoolDefinition[];
	principals?: PrincipalDefinition[];
	processes?: ProcessDefinition[];
	scheduledJobs?: ScheduledJobDefinition[];
	/**
	 * OpenAPI document (OpenAPI 3.x or Swagger 2.x) for this application,
	 * as parsed JSON. Optional — only include if you want the platform to
	 * track your REST surface in its catalogue. Each sync replaces the
	 * previously published version.
	 */
	openapiSpec?: unknown;
}

/**
 * Fluent builder for `DefinitionSet`. Mirrors the Laravel SDK's
 * `SyncDefinitionSet` shape so definitions can be described the same way
 * across languages.
 *
 * Example:
 * ```ts
 * const set = defineApplication("orders")
 *   .withRoles([{ name: "admin", displayName: "Administrator" }])
 *   .withEventTypes([
 *     { code: "orders:fulfillment:shipment:shipped", name: "Shipment Shipped" },
 *   ])
 *   .build();
 * ```
 */
export class DefinitionSetBuilder {
	private readonly set: DefinitionSet;

	constructor(applicationCode: string) {
		this.set = { applicationCode };
	}

	withRoles(roles: RoleDefinition[]): this {
		this.set.roles = [...(this.set.roles ?? []), ...roles];
		return this;
	}

	withEventTypes(eventTypes: EventTypeDefinition[]): this {
		this.set.eventTypes = [...(this.set.eventTypes ?? []), ...eventTypes];
		return this;
	}

	withSubscriptions(subscriptions: SubscriptionDefinition[]): this {
		this.set.subscriptions = [
			...(this.set.subscriptions ?? []),
			...subscriptions,
		];
		return this;
	}

	withDispatchPools(pools: DispatchPoolDefinition[]): this {
		this.set.dispatchPools = [...(this.set.dispatchPools ?? []), ...pools];
		return this;
	}

	withPrincipals(principals: PrincipalDefinition[]): this {
		this.set.principals = [...(this.set.principals ?? []), ...principals];
		return this;
	}

	withProcesses(processes: ProcessDefinition[]): this {
		this.set.processes = [...(this.set.processes ?? []), ...processes];
		return this;
	}

	withScheduledJobs(jobs: ScheduledJobDefinition[]): this {
		this.set.scheduledJobs = [...(this.set.scheduledJobs ?? []), ...jobs];
		return this;
	}

	/**
	 * Attach an OpenAPI document (parsed JSON) to be published alongside
	 * the rest of the application's definitions on next sync.
	 */
	withOpenapiSpec(spec: unknown): this {
		this.set.openapiSpec = spec;
		return this;
	}

	build(): DefinitionSet {
		return { ...this.set };
	}
}

/** Convenience: start building definitions for `applicationCode`. */
export function defineApplication(applicationCode: string): DefinitionSetBuilder {
	return new DefinitionSetBuilder(applicationCode);
}
