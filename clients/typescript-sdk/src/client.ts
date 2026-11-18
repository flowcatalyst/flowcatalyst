/**
 * FlowCatalyst SDK Client
 *
 * Main client for interacting with the FlowCatalyst platform.
 * Uses neverthrow for typed error handling.
 */

import { ResultAsync, ok, err, errAsync, type Result } from "neverthrow";
import { createClient, createConfig } from "./generated/client/index.js";
import type { Client } from "./generated/client/index.js";
import { OidcTokenManager, type TokenManagerConfig } from "./auth.js";
import type { SdkError } from "./errors.js";
import {
	mapHttpStatusToError,
	httpError,
	authError,
	outcomeUnknownError,
} from "./errors.js";

// Re-export resource classes
import { AuditLogsResource } from "./resources/audit-logs.js";
import { EventTypesResource } from "./resources/event-types.js";
import { ProcessesResource } from "./resources/processes.js";
import { SubscriptionsResource } from "./resources/subscriptions.js";
import { DispatchPoolsResource } from "./resources/dispatch-pools.js";
import { RolesResource } from "./resources/roles.js";
import { PermissionsResource } from "./resources/permissions.js";
import { ApplicationsResource } from "./resources/applications.js";
import { ClientsResource } from "./resources/clients.js";
import { PrincipalsResource } from "./resources/principals.js";
import { MeResource } from "./resources/me.js";
import { ConnectionsResource } from "./resources/connections.js";
import { PlatformConfigResource } from "./resources/platform-config.js";
import {
	DefinitionSynchronizer,
	type DefinitionSynchronizerOptions,
} from "./sync/definition-synchronizer.js";
import { RouterResource } from "./resources/router.js";
import { IngestionsResource } from "./resources/ingestions.js";
import { FieldWorkersResource } from "./resources/field-workers.js";
import { FieldDevicesResource } from "./resources/field-devices.js";
import { FieldPoliciesResource } from "./resources/field-policies.js";
import { FieldTokensResource } from "./resources/field-tokens.js";
import { ScheduledJobsResource } from "./resources/scheduled-jobs.js";
import {
	ScheduledJobRunner,
	type RunnerOptions,
} from "./runner/scheduled-job-runner.js";

/**
 * Configuration for client credentials authentication.
 */
export interface ClientCredentialsConfig extends TokenManagerConfig {
	/** Request timeout in milliseconds (default: 30000) */
	timeout?: number;
	/** Number of retry attempts for transient errors (default: 3) */
	retryAttempts?: number;
	/** Base delay between retries in ms (default: 100) */
	retryDelay?: number;
	/**
	 * Base URL of the message router for monitoring endpoints
	 * (`/monitoring/in-flight-messages/...`). The router runs at a
	 * different host than the platform; if not set, router calls fall
	 * back to `baseUrl` (correct only when router and platform share a
	 * host, e.g. `fcdev`).
	 */
	routerBaseUrl?: string;
}

/**
 * Configuration for user token authentication.
 * Use this when you already have a user access token (e.g., from login).
 */
export interface UserTokenConfig {
	/** Base URL of the FlowCatalyst API */
	baseUrl: string;
	/**
	 * Access token or a function that returns the current access token.
	 * Use a function if the token may change (e.g., refreshed by your app).
	 */
	accessToken: string | (() => string) | (() => Promise<string>);
	/** Request timeout in milliseconds (default: 30000) */
	timeout?: number;
	/** Number of retry attempts for transient errors (default: 3) */
	retryAttempts?: number;
	/** Base delay between retries in ms (default: 100) */
	retryDelay?: number;
	/**
	 * Base URL of the message router for monitoring endpoints. See
	 * `ClientCredentialsConfig.routerBaseUrl`.
	 */
	routerBaseUrl?: string;
}

export type FlowCatalystConfig = ClientCredentialsConfig | UserTokenConfig;

function isUserTokenConfig(
	config: FlowCatalystConfig,
): config is UserTokenConfig {
	return "accessToken" in config;
}

type ResolvedConfig = {
	baseUrl: string;
	routerBaseUrl: string;
	timeout: number;
	retryAttempts: number;
	retryDelay: number;
};

type TokenProvider = () => Promise<string>;

/**
 * Per-call options of {@link FlowCatalystClient.request}.
 */
export interface RequestOptions {
	/**
	 * The request may be sent again after its answer was lost (a 502/503/504,
	 * or a connection failure or timeout after it was sent): the platform
	 * applies it once however often it arrives. Only for a POST that is: a
	 * declarative sync, a read-only POST, a batch whose every item carries
	 * the key the platform de-duplicates by. GET, HEAD, OPTIONS, PUT and
	 * DELETE are repeatable without it.
	 */
	repeatable?: boolean;
	/** The method, for a `fn` that calls `fetch` itself. */
	method?: string;
	/** The path, for a `fn` that calls `fetch` itself (used in errors). */
	path?: string;
}

/** What one attempt sent, recorded by the per-request fetch. */
type SentRequest = { method?: string; url?: string };

/** Statuses that say the request was not processed: retried for every request. */
const REFUSED_UNPROCESSED = [408, 429];

/** Gateway statuses after which nobody can say whether the request was applied. */
const GATEWAY_UNKNOWN = [502, 503, 504];

const REPEATABLE_METHODS = ["GET", "HEAD", "OPTIONS", "PUT", "DELETE"];

function isRepeatable(method: string, options: RequestOptions): boolean {
	return options.repeatable === true || REPEATABLE_METHODS.includes(method);
}

/**
 * Codes of a connection that was never made, so no byte of the request was
 * written (Node's fetch puts them on the error's `cause`, or on each of an
 * `AggregateError`'s errors when several addresses were tried).
 */
const CONNECTION_NOT_MADE = new Set([
	"ECONNREFUSED",
	"ENOTFOUND",
	"EAI_AGAIN",
	"EHOSTUNREACH",
	"ENETUNREACH",
	"UND_ERR_CONNECT_TIMEOUT",
]);

function connectionNotMade(e: unknown): boolean {
	const codes: string[] = [];
	const collect = (x: unknown, depth: number): void => {
		if (typeof x !== "object" || x === null || depth > 4) return;
		const code = (x as { code?: unknown }).code;
		if (typeof code === "string") codes.push(code);
		const errors = (x as { errors?: unknown }).errors;
		if (Array.isArray(errors)) for (const inner of errors) collect(inner, depth + 1);
		collect((x as { cause?: unknown }).cause, depth + 1);
	};
	collect(e, 0);
	return codes.length > 0 && codes.every((c) => CONNECTION_NOT_MADE.has(c));
}

/**
 * Main FlowCatalyst SDK client.
 *
 * @example
 * ```typescript
 * import { FlowCatalystClient } from '@flowcatalyst/sdk';
 *
 * const client = new FlowCatalystClient({
 *   baseUrl: 'https://your-instance.flowcatalyst.io',
 *   clientId: 'your_client_id',
 *   clientSecret: 'your_client_secret',
 * });
 *
 * // Using neverthrow Result pattern
 * const result = await client.eventTypes().list();
 * result.match(
 *   (eventTypes) => console.log('Event types:', eventTypes),
 *   (error) => console.error('Error:', error.type, error.message)
 * );
 * ```
 */
export class FlowCatalystClient {
	private readonly httpClient: Client;
	private readonly tokenManager: OidcTokenManager | null;
	private readonly tokenProvider: TokenProvider | null;
	private readonly config: ResolvedConfig;

	// Lazy-loaded resource instances
	private _eventTypes?: EventTypesResource;
	private _processes?: ProcessesResource;
	private _subscriptions?: SubscriptionsResource;
	private _dispatchPools?: DispatchPoolsResource;
	private _roles?: RolesResource;
	private _permissions?: PermissionsResource;
	private _applications?: ApplicationsResource;
	private _clients?: ClientsResource;
	private _principals?: PrincipalsResource;
	private _me?: MeResource;
	private _connections?: ConnectionsResource;
	private _platformConfig?: PlatformConfigResource;
	private _definitions?: DefinitionSynchronizer;
	private _router?: RouterResource;
	private _scheduledJobs?: ScheduledJobsResource;
	private _auditLogs?: AuditLogsResource;
	private _ingestions?: IngestionsResource;
	private _fieldWorkers?: FieldWorkersResource;
	private _fieldDevices?: FieldDevicesResource;
	private _fieldPolicies?: FieldPoliciesResource;
	private _fieldTokens?: FieldTokensResource;

	constructor(config: FlowCatalystConfig) {
		this.config = {
			timeout: config.timeout ?? 30000,
			retryAttempts: config.retryAttempts ?? 3,
			retryDelay: config.retryDelay ?? 100,
			baseUrl: config.baseUrl.replace(/\/$/, ""),
			routerBaseUrl: (config.routerBaseUrl ?? config.baseUrl).replace(
				/\/$/,
				"",
			),
		};

		if (isUserTokenConfig(config)) {
			// User token mode - use provided token or token provider
			this.tokenManager = null;
			const tokenOrFn = config.accessToken;
			if (typeof tokenOrFn === "string") {
				// Static token
				this.tokenProvider = () => Promise.resolve(tokenOrFn);
			} else {
				// Function that returns token (sync or async)
				this.tokenProvider = async () => tokenOrFn();
			}
		} else {
			// Client credentials mode - use token manager
			this.tokenManager = new OidcTokenManager({
				baseUrl: this.config.baseUrl,
				clientId: config.clientId,
				clientSecret: config.clientSecret,
				tokenUrl: config.tokenUrl,
			});
			this.tokenProvider = null;
		}

		this.httpClient = createClient(
			createConfig({
				baseUrl: this.config.baseUrl,
			}),
		);
	}

	// ============ Resource Accessors ============

	/** Event Types resource */
	eventTypes(): EventTypesResource {
		return (this._eventTypes ??= new EventTypesResource(this));
	}

	/** Processes resource (workflow documentation / Mermaid diagrams) */
	processes(): ProcessesResource {
		return (this._processes ??= new ProcessesResource(this));
	}

	/** Subscriptions resource */
	subscriptions(): SubscriptionsResource {
		return (this._subscriptions ??= new SubscriptionsResource(this));
	}

	/** Dispatch Pools resource */
	dispatchPools(): DispatchPoolsResource {
		return (this._dispatchPools ??= new DispatchPoolsResource(this));
	}

	/** Roles resource */
	roles(): RolesResource {
		return (this._roles ??= new RolesResource(this));
	}

	/** Permissions resource */
	permissions(): PermissionsResource {
		return (this._permissions ??= new PermissionsResource(this));
	}

	/** Applications resource */
	applications(): ApplicationsResource {
		return (this._applications ??= new ApplicationsResource(this));
	}

	/** Clients resource */
	clients(): ClientsResource {
		return (this._clients ??= new ClientsResource(this));
	}

	/** Principals resource */
	principals(): PrincipalsResource {
		return (this._principals ??= new PrincipalsResource(this));
	}

	/** Me resource (user-scoped access to clients and applications) */
	me(): MeResource {
		return (this._me ??= new MeResource(this));
	}

	/** Connections resource */
	connections(): ConnectionsResource {
		return (this._connections ??= new ConnectionsResource(this));
	}

	/** Platform config resource (properties and SECRET reveal) */
	platformConfig(): PlatformConfigResource {
		return (this._platformConfig ??= new PlatformConfigResource(this));
	}

	/**
	 * Message router monitoring resource. Provides presence checks
	 * against the router's in-pipeline map (single id and batch).
	 * Talks to `routerBaseUrl` if configured, otherwise falls back to
	 * `baseUrl`.
	 */
	router(): RouterResource {
		return (this._router ??= new RouterResource(this));
	}

	/** Scheduled Jobs resource (CRUD + state transitions + history reads). */
	scheduledJobs(): ScheduledJobsResource {
		return (this._scheduledJobs ??= new ScheduledJobsResource(this));
	}

	/** Audit Logs resource (read-only queries against `iam_audit_logs`). */
	auditLogs(): AuditLogsResource {
		return (this._auditLogs ??= new AuditLogsResource(this));
	}

	/** Ingestions resource (definitions, versions, test connection, sets). */
	ingestions(): IngestionsResource {
		return (this._ingestions ??= new IngestionsResource(this));
	}

	/** Field workers resource (admin API for PIN-based field identities). */
	fieldWorkers(): FieldWorkersResource {
		return (this._fieldWorkers ??= new FieldWorkersResource(this));
	}

	/** Field devices resource (enrolment codes and enrolled devices). */
	fieldDevices(): FieldDevicesResource {
		return (this._fieldDevices ??= new FieldDevicesResource(this));
	}

	/** Field policies resource (per-client PIN, lock and session policy). */
	fieldPolicies(): FieldPoliciesResource {
		return (this._fieldPolicies ??= new FieldPoliciesResource(this));
	}

	/** Field tokens resource (online introspection). */
	fieldTokens(): FieldTokensResource {
		return (this._fieldTokens ??= new FieldTokensResource(this));
	}

	/**
	 * Online check of a field access token (`POST /api/field-tokens/introspect`).
	 * Use when the offline `FieldTokenVerifier` is not enough and the session
	 * must still be live.
	 */
	introspectFieldToken(token: string) {
		return this.fieldTokens().introspect(token);
	}

	/**
	 * Build a `ScheduledJobRunner` you can mount on your HTTP framework.
	 * Register handlers per job code, then call
	 * `.processSigned(rawBody, headers)` from your route handler (pass
	 * `signingSecret` in `options` so firings are signature-verified;
	 * `.process(envelope)` is the unverified path). The runner enforces single-instance concurrency
	 * via the injected `LockProvider` (default: NoOp — provide a real one
	 * for production).
	 */
	scheduledJobRunner(options?: RunnerOptions): ScheduledJobRunner {
		return new ScheduledJobRunner(this, this.scheduledJobs(), options);
	}

	/** Base URL of the message router (used by `router()`). */
	getRouterBaseUrl(): string {
		return this.config.routerBaseUrl;
	}

	/**
	 * Definition synchronizer — bulk-sync roles, event types, connections,
	 * subscriptions, dispatch pools, and principals per application.
	 *
	 * `options` (e.g. `subscriptionTargetBaseUrl`) only take effect on the
	 * FIRST call — the synchronizer is created once and cached.
	 *
	 * See `docs/syncing-definitions.md` for structure and conventions.
	 */
	definitions(options?: DefinitionSynchronizerOptions): DefinitionSynchronizer {
		return (this._definitions ??= new DefinitionSynchronizer(this, options));
	}

	// ============ Internal Methods ============

	/**
	 * Get the underlying HTTP client (for advanced usage).
	 */
	getHttpClient(): Client {
		return this.httpClient;
	}

	/**
	 * Get the token manager (for advanced usage).
	 * Returns null if using user token mode.
	 */
	getTokenManager(): OidcTokenManager | null {
		return this.tokenManager;
	}

	/**
	 * Execute an authenticated request with retry logic.
	 *
	 * Wraps the generated SDK functions with:
	 * - Automatic token injection
	 * - Retry with exponential backoff, of what is safe to repeat only
	 * - Typed error handling via neverthrow
	 *
	 * A request is repeatable when its method is GET, HEAD, OPTIONS, PUT or
	 * DELETE, or when `options.repeatable` marks it (a POST the platform
	 * applies once however often it is sent: the declarative syncs, read-only
	 * POSTs). Then, up to `retryAttempts` retries:
	 *
	 * - 408 and 429 (refused before it was processed) are retried for every
	 *   request; so is a connection that could not be made (DNS failure,
	 *   refused, connect timeout), since nothing was sent.
	 * - 502, 503 and 504, and a connection failure or timeout after the
	 *   request was sent, are retried for a repeatable request. For any other
	 *   request they end the call with an `outcome_unknown` error at once: the
	 *   platform may have applied it, and sending it again could apply it twice
	 *   (or, for a provisioning, be refused with the one-time secrets lost).
	 * - A DELETE that answers 404 on a retry after an attempt whose outcome was
	 *   unknown succeeds: the earlier attempt most likely deleted it.
	 *
	 * `options.method` names the method for a `fn` that calls `fetch` itself
	 * rather than through the client it is given; without either, a request
	 * counts as not repeatable.
	 */
	request<T>(
		fn: (
			client: Client,
			headers: Record<string, string>,
		) => Promise<{ data?: unknown; error?: unknown; response: Response }>,
		options: RequestOptions = {},
	): ResultAsync<T, SdkError> {
		if (this.tokenProvider) {
			// User token mode
			return ResultAsync.fromPromise(this.tokenProvider(), (e) =>
				authError.tokenExpired(
					e instanceof Error ? e.message : "Failed to get access token",
				),
			).andThen((token) => this.executeWithRetry<T>(fn, token, false, options));
		} else if (this.tokenManager) {
			// Client credentials mode
			return this.tokenManager
				.getAccessToken()
				.mapErr((e): SdkError => e)
				.andThen((token) => this.executeWithRetry<T>(fn, token, true, options));
		} else {
			return errAsync(authError.tokenExpired("No authentication configured"));
		}
	}

	private executeWithRetry<T>(
		fn: (
			client: Client,
			headers: Record<string, string>,
		) => Promise<{ data?: unknown; error?: unknown; response: Response }>,
		token: string,
		canRefreshToken: boolean,
		options: RequestOptions,
	): ResultAsync<T, SdkError> {
		return new ResultAsync(this.runWithRetry<T>(fn, token, canRefreshToken, options));
	}

	private async runWithRetry<T>(
		fn: (
			client: Client,
			headers: Record<string, string>,
		) => Promise<{ data?: unknown; error?: unknown; response: Response }>,
		initialToken: string,
		canRefreshToken: boolean,
		options: RequestOptions,
	): Promise<Result<T, SdkError>> {
		let token = initialToken;
		let refreshed = false;
		let retries = 0;
		// An earlier attempt may have been applied by the platform.
		let uncertain = false;
		const retry = async (): Promise<boolean> => {
			if (retries >= this.config.retryAttempts) return false;
			await this.delay(this.config.retryDelay * Math.pow(2, retries));
			retries++;
			return true;
		};

		for (;;) {
			const sent: SentRequest = {};
			let result: { data?: unknown; error?: unknown; response: Response };
			try {
				result = await this.executeWithTimeout(
					fn,
					{ Authorization: `Bearer ${token}` },
					sent,
				);
			} catch (e) {
				const timedOut = e instanceof Error && e.name === "AbortError";
				const failure = timedOut
					? httpError.timeout(this.config.timeout)
					: httpError.network(
							e instanceof Error ? e.message : "Network error",
							e instanceof Error ? e : undefined,
						);
				if (!timedOut && connectionNotMade(e)) {
					// Nothing was sent: safe to try again whatever it is.
					if (await retry()) continue;
					return err(failure);
				}
				const { method, url } = this.describe(sent, options);
				if (!isRepeatable(method, options)) {
					return err(
						outcomeUnknownError(method, url, {
							cause: e instanceof Error ? e : new Error(failure.message),
						}),
					);
				}
				uncertain = true;
				if (await retry()) continue;
				return err(failure);
			}

			if (result.error === undefined && result.response.ok) {
				return ok(result.data as T);
			}
			const status = result.response.status;

			// Handle 401 with token refresh (only for client credentials mode)
			if (
				status === 401 &&
				!refreshed &&
				retries === 0 &&
				canRefreshToken &&
				this.tokenManager
			) {
				refreshed = true;
				const fresh = await this.tokenManager.refreshToken();
				if (fresh.isErr()) return err(fresh.error);
				token = fresh.value;
				continue;
			}

			const { method, url } = this.describe(sent, options);
			if (uncertain && method === "DELETE" && status === 404) {
				// The attempt whose answer was lost most likely deleted it.
				return ok(undefined as T);
			}
			if (REFUSED_UNPROCESSED.includes(status)) {
				if (await retry()) continue;
			} else if (GATEWAY_UNKNOWN.includes(status)) {
				if (!isRepeatable(method, options)) {
					return err(
						outcomeUnknownError(method, url, { status, body: result.error }),
					);
				}
				uncertain = true;
				if (await retry()) continue;
			}
			return err(mapHttpStatusToError(status, result.error));
		}
	}

	/** The method and URL of the attempt, as sent or as the caller named them. */
	private describe(
		sent: SentRequest,
		options: RequestOptions,
	): { method: string; url: string } {
		return {
			method: (sent.method ?? options.method ?? "UNKNOWN").toUpperCase(),
			url: sent.url ?? options.path ?? this.config.baseUrl,
		};
	}

	private async executeWithTimeout(
		fn: (
			client: Client,
			headers: Record<string, string>,
		) => Promise<{ data?: unknown; error?: unknown; response: Response }>,
		headers: Record<string, string>,
		sent: SentRequest,
	): Promise<{ data?: unknown; error?: unknown; response: Response }> {
		const controller = new AbortController();
		const timeoutId = setTimeout(() => controller.abort(), this.config.timeout);

		// A per-request client whose fetch carries the timeout signal, so the
		// timer actually aborts the in-flight request (the shared client has
		// no per-call signal hook), and records what was sent (the retry rule
		// needs the method).
		const signal = controller.signal;
		const httpClient = createClient(
			createConfig({
				baseUrl: this.config.baseUrl,
				fetch: (input: RequestInfo | URL, init?: RequestInit) => {
					if (typeof input === "object" && "method" in input) {
						sent.method = input.method;
						sent.url = input.url;
					} else {
						sent.method = init?.method ?? "GET";
						sent.url = String(input);
					}
					return globalThis.fetch(input, { ...init, signal });
				},
			}),
		);

		try {
			const result = await fn(httpClient, headers);
			// The generated client reports fetch exceptions (abort, network
			// failure) as `{ error, response: undefined }` instead of
			// throwing; rethrow so the caller maps them to timeout/network
			// errors rather than dereferencing a missing response.
			if (result.response === undefined) {
				if (signal.aborted) {
					const abort = new Error("The operation was aborted");
					abort.name = "AbortError";
					throw abort;
				}
				throw result.error instanceof Error
					? result.error
					: new Error("Network error");
			}
			return result;
		} finally {
			clearTimeout(timeoutId);
		}
	}

	private delay(ms: number): Promise<void> {
		return new Promise((resolve) => setTimeout(resolve, ms));
	}
}
