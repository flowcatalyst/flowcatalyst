/**
 * DefinitionSynchronizer — orchestrates syncing a `DefinitionSet` to the
 * platform's application-scoped sync API (`/api/applications/{app}/*\/sync`).
 *
 * One orchestrator per `FlowCatalystClient`; auth/retry/errors are delegated
 * to the client's shared request pipeline.
 */

import { okAsync, type ResultAsync } from "neverthrow";
import type { FlowCatalystClient } from "../client.js";
import type { SdkError } from "../errors.js";
import type {
	DefinitionSet,
	DispatchPoolDefinition,
	EventTypeDefinition,
	PrincipalDefinition,
	ProcessDefinition,
	RoleDefinition,
	ScheduledJobDefinition,
	SubscriptionDefinition,
} from "./definitions.js";
import { permissionToString } from "./definitions.js";
import type {
	CategorySyncResult,
	MaybeCategoryResult,
	SyncResult,
} from "./result.js";
import { SKIPPED } from "./result.js";

/** Options for a sync call. */
export interface SyncOptions {
	/**
	 * When true, the platform removes SDK-sourced rows not present in the
	 * submitted list (per category). Rows created through the admin UI are
	 * preserved regardless. Default: false.
	 */
	removeUnlisted?: boolean;
	/**
	 * Per-category opt-out. Omitting a category from the `DefinitionSet`
	 * already skips it; these flags let you force-skip categories even if
	 * they're present (e.g. to stage a rollout).
	 */
	skipRoles?: boolean;
	skipEventTypes?: boolean;
	skipSubscriptions?: boolean;
	skipDispatchPools?: boolean;
	skipPrincipals?: boolean;
	skipProcesses?: boolean;
	skipScheduledJobs?: boolean;
	/** When true, skip publishing the OpenAPI doc even if the set has one. */
	skipOpenapi?: boolean;
}

/**
 * Sync FlowCatalyst definitions to the platform.
 *
 * Construct via `client.definitions()`; the orchestrator reuses the
 * client's auth, retry, and error handling.
 */
export class DefinitionSynchronizer {
	private readonly client: FlowCatalystClient;

	constructor(client: FlowCatalystClient) {
		this.client = client;
	}

	/**
	 * Sync one application's definitions.
	 *
	 * Categories are sync'd in a fixed order — roles, event types,
	 * subscriptions, dispatch pools, principals — so that subscriptions
	 * can reference the event types and dispatch pools that were just
	 * created. Each category sync is an independent HTTP call; a failure
	 * in one does NOT roll back earlier successes.
	 */
	sync(
		set: DefinitionSet,
		options: SyncOptions = {},
	): ResultAsync<SyncResult, SdkError> {
		const removeUnlisted = options.removeUnlisted ?? false;

		const rolesStep: () => ResultAsync<MaybeCategoryResult, SdkError> = () =>
			options.skipRoles || !set.roles
				? okAsync<MaybeCategoryResult>(SKIPPED)
				: this.syncRoles(set.applicationCode, set.roles, removeUnlisted);
		const eventTypesStep: () => ResultAsync<MaybeCategoryResult, SdkError> = () =>
			options.skipEventTypes || !set.eventTypes
				? okAsync<MaybeCategoryResult>(SKIPPED)
				: this.syncEventTypes(
						set.applicationCode,
						set.eventTypes,
						removeUnlisted,
					);
		const subsStep: () => ResultAsync<MaybeCategoryResult, SdkError> = () =>
			options.skipSubscriptions || !set.subscriptions
				? okAsync<MaybeCategoryResult>(SKIPPED)
				: this.syncSubscriptions(
						set.applicationCode,
						set.subscriptions,
						removeUnlisted,
					);
		const poolsStep: () => ResultAsync<MaybeCategoryResult, SdkError> = () =>
			options.skipDispatchPools || !set.dispatchPools
				? okAsync<MaybeCategoryResult>(SKIPPED)
				: this.syncDispatchPools(
						set.applicationCode,
						set.dispatchPools,
						removeUnlisted,
					);
		const principalsStep: () => ResultAsync<MaybeCategoryResult, SdkError> =
			() =>
				options.skipPrincipals || !set.principals
					? okAsync<MaybeCategoryResult>(SKIPPED)
					: this.syncPrincipals(
							set.applicationCode,
							set.principals,
							removeUnlisted,
						);
		const processesStep: () => ResultAsync<MaybeCategoryResult, SdkError> =
			() =>
				options.skipProcesses || !set.processes
					? okAsync<MaybeCategoryResult>(SKIPPED)
					: this.syncProcesses(
							set.applicationCode,
							set.processes,
							removeUnlisted,
						);
		const scheduledJobsStep: () => ResultAsync<MaybeCategoryResult, SdkError> =
			() =>
				options.skipScheduledJobs || !set.scheduledJobs
					? okAsync<MaybeCategoryResult>(SKIPPED)
					: this.syncScheduledJobs(
							set.applicationCode,
							set.scheduledJobs,
							removeUnlisted,
						);
		const openapiStep: () => ResultAsync<MaybeCategoryResult, SdkError> = () =>
			options.skipOpenapi || set.openapiSpec === undefined
				? okAsync<MaybeCategoryResult>(SKIPPED)
				: this.syncOpenapi(set.applicationCode, set.openapiSpec);

		return rolesStep()
			.andThen((roles) =>
				eventTypesStep().map((eventTypes) => ({ roles, eventTypes })),
			)
			.andThen((acc) =>
				subsStep().map((subscriptions) => ({ ...acc, subscriptions })),
			)
			.andThen((acc) =>
				poolsStep().map((dispatchPools) => ({ ...acc, dispatchPools })),
			)
			.andThen((acc) =>
				principalsStep().map((principals) => ({ ...acc, principals })),
			)
			.andThen((acc) =>
				processesStep().map((processes) => ({ ...acc, processes })),
			)
			.andThen((acc) =>
				scheduledJobsStep().map((scheduledJobs) => ({ ...acc, scheduledJobs })),
			)
			.andThen((acc) =>
				openapiStep().map(
					(openapi): SyncResult => ({
						applicationCode: set.applicationCode,
						...acc,
						openapi,
					}),
				),
			);
	}

	/**
	 * Sync multiple applications' definitions. Each set is sync'd
	 * sequentially; results are returned in the same order as `sets`.
	 * A failure in one set short-circuits the rest.
	 */
	syncAll(
		sets: DefinitionSet[],
		options: SyncOptions = {},
	): ResultAsync<SyncResult[], SdkError> {
		return sets.reduce<ResultAsync<SyncResult[], SdkError>>(
			(chain, set) =>
				chain.andThen((acc) =>
					this.sync(set, options).map((result) => [...acc, result]),
				),
			okAsync<SyncResult[]>([]),
		);
	}

	// ── per-category callers ──────────────────────────────────────────

	private syncRoles(
		applicationCode: string,
		roles: RoleDefinition[],
		removeUnlisted: boolean,
	): ResultAsync<CategorySyncResult, SdkError> {
		// Resolve any PermissionInput factories to strings (idempotent on
		// strings), so a set posted without build() still sends the wire shape.
		const resolved = roles.map((role) =>
			role.permissions
				? {
						...role,
						permissions: role.permissions.map((p) =>
							permissionToString(p, applicationCode),
						),
					}
				: role,
		);
		return this.post(applicationCode, "roles", { roles: resolved }, removeUnlisted);
	}

	private syncEventTypes(
		applicationCode: string,
		eventTypes: EventTypeDefinition[],
		removeUnlisted: boolean,
	): ResultAsync<CategorySyncResult, SdkError> {
		return this.post(
			applicationCode,
			"event-types",
			{ eventTypes },
			removeUnlisted,
		);
	}

	private syncSubscriptions(
		applicationCode: string,
		subscriptions: SubscriptionDefinition[],
		removeUnlisted: boolean,
	): ResultAsync<CategorySyncResult, SdkError> {
		return this.post(
			applicationCode,
			"subscriptions",
			{ subscriptions },
			removeUnlisted,
		);
	}

	private syncDispatchPools(
		applicationCode: string,
		pools: DispatchPoolDefinition[],
		removeUnlisted: boolean,
	): ResultAsync<CategorySyncResult, SdkError> {
		return this.post(
			applicationCode,
			"dispatch-pools",
			{ pools },
			removeUnlisted,
		);
	}

	private syncPrincipals(
		applicationCode: string,
		principals: PrincipalDefinition[],
		removeUnlisted: boolean,
	): ResultAsync<CategorySyncResult, SdkError> {
		return this.post(
			applicationCode,
			"principals",
			{ principals },
			removeUnlisted,
		);
	}

	private syncProcesses(
		applicationCode: string,
		processes: ProcessDefinition[],
		removeUnlisted: boolean,
	): ResultAsync<CategorySyncResult, SdkError> {
		return this.post(
			applicationCode,
			"processes",
			{ processes },
			removeUnlisted,
		);
	}

	private syncScheduledJobs(
		applicationCode: string,
		jobs: ScheduledJobDefinition[],
		removeUnlisted: boolean,
	): ResultAsync<CategorySyncResult, SdkError> {
		// Scheduled-jobs sync is the one endpoint that uses `archiveUnlisted`
		// in the body rather than `removeUnlisted` as a query param. Keep
		// the SDK API consistent — caller passes `removeUnlisted`, we
		// translate at the wire.
		return this.client.request<CategorySyncResult>((httpClient, headers) =>
			httpClient.post({
				url: `/api/applications/${applicationCode}/scheduled-jobs/sync`,
				headers: { ...headers, "Content-Type": "application/json" },
				body: {
					jobs,
					archiveUnlisted: removeUnlisted,
				},
			}),
		);
	}

	private syncOpenapi(
		applicationCode: string,
		spec: unknown,
	): ResultAsync<CategorySyncResult, SdkError> {
		// OpenAPI sync is one-shot — body is `{ spec }`, not a list.
		// The platform's response has a different shape; we normalise to
		// CategorySyncResult so callers can iterate uniformly.
		return this.client
			.request<{
				applicationCode: string;
				version: string;
				archivedPriorVersion?: string;
				unchanged: boolean;
			}>((httpClient, headers) =>
				httpClient.post({
					url: `/api/applications/${applicationCode}/openapi/sync`,
					headers: { ...headers, "Content-Type": "application/json" },
					body: { spec },
				}),
			)
			.map((r) => {
				const created = r.unchanged || r.archivedPriorVersion ? 0 : 1;
				const updated = r.archivedPriorVersion ? 1 : 0;
				return {
					applicationCode: r.applicationCode,
					created,
					updated,
					deleted: 0,
					syncedCodes: [r.version],
				} satisfies CategorySyncResult;
			});
	}

	// ── transport ─────────────────────────────────────────────────────

	private post(
		applicationCode: string,
		resource: string,
		body: Record<string, unknown>,
		removeUnlisted: boolean,
	): ResultAsync<CategorySyncResult, SdkError> {
		return this.client.request<CategorySyncResult>((httpClient, headers) =>
			httpClient.post({
				url: `/api/applications/${applicationCode}/${resource}/sync`,
				headers: {
					...headers,
					"Content-Type": "application/json",
				},
				body,
				query: { removeUnlisted },
			}),
		);
	}
}

