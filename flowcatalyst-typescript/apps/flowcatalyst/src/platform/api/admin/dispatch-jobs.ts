/**
 * Dispatch Jobs BFF API
 *
 * Read-only endpoints for browsing dispatch jobs from the dispatch_jobs_read projection.
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import { jsonSuccess, notFound, ErrorResponseSchema } from "@flowcatalyst/http";
import type {
	DispatchJobReadRecord,
	DispatchJobAttemptRecord,
} from "@flowcatalyst/persistence";

import type { DispatchJobReadRepository } from "../../infrastructure/persistence/index.js";
import {
	requirePermission,
	getAccessibleClientIds,
	canAccessResourceByClient,
} from "../../authorization/index.js";
import { DISPATCH_JOB_PERMISSIONS } from "../../authorization/permissions/platform-admin.js";

// ─── Param Schemas ──────────────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });

// ─── Query Schemas ──────────────────────────────────────────────────────────

const DispatchJobListQuerySchema = Type.Object({
	clientIds: Type.Optional(Type.String()),
	statuses: Type.Optional(Type.String()),
	applications: Type.Optional(Type.String()),
	subdomains: Type.Optional(Type.String()),
	aggregates: Type.Optional(Type.String()),
	codes: Type.Optional(Type.String()),
	source: Type.Optional(Type.String()),
	kind: Type.Optional(Type.String()),
	subscriptionId: Type.Optional(Type.String()),
	dispatchPoolId: Type.Optional(Type.String()),
	messageGroup: Type.Optional(Type.String()),
	createdAfter: Type.Optional(Type.String()),
	createdBefore: Type.Optional(Type.String()),
	page: Type.Optional(Type.String()),
	size: Type.Optional(Type.String()),
});

const FilterOptionsQuerySchema = Type.Object({
	clientIds: Type.Optional(Type.String()),
	applications: Type.Optional(Type.String()),
	subdomains: Type.Optional(Type.String()),
	aggregates: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const DispatchJobReadResponseSchema = Type.Object({
	id: Type.String(),
	externalId: Type.Union([Type.String(), Type.Null()]),
	source: Type.Union([Type.String(), Type.Null()]),
	kind: Type.String(),
	code: Type.String(),
	subject: Type.Union([Type.String(), Type.Null()]),
	eventId: Type.Union([Type.String(), Type.Null()]),
	correlationId: Type.Union([Type.String(), Type.Null()]),
	targetUrl: Type.String(),
	protocol: Type.String(),
	application: Type.Union([Type.String(), Type.Null()]),
	subdomain: Type.Union([Type.String(), Type.Null()]),
	aggregate: Type.Union([Type.String(), Type.Null()]),
	clientId: Type.Union([Type.String(), Type.Null()]),
	subscriptionId: Type.Union([Type.String(), Type.Null()]),
	serviceAccountId: Type.Union([Type.String(), Type.Null()]),
	dispatchPoolId: Type.Union([Type.String(), Type.Null()]),
	messageGroup: Type.Union([Type.String(), Type.Null()]),
	mode: Type.String(),
	sequence: Type.Union([Type.Integer(), Type.Null()]),
	status: Type.String(),
	attemptCount: Type.Integer(),
	maxRetries: Type.Integer(),
	lastError: Type.Union([Type.String(), Type.Null()]),
	timeoutSeconds: Type.Union([Type.Integer(), Type.Null()]),
	retryStrategy: Type.Union([Type.String(), Type.Null()]),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
	scheduledFor: Type.Union([Type.String({ format: "date-time" }), Type.Null()]),
	expiresAt: Type.Union([Type.String({ format: "date-time" }), Type.Null()]),
	completedAt: Type.Union([Type.String({ format: "date-time" }), Type.Null()]),
	lastAttemptAt: Type.Union([
		Type.String({ format: "date-time" }),
		Type.Null(),
	]),
	durationMillis: Type.Union([Type.Integer(), Type.Null()]),
	idempotencyKey: Type.Union([Type.String(), Type.Null()]),
	isCompleted: Type.Union([Type.Boolean(), Type.Null()]),
	isTerminal: Type.Union([Type.Boolean(), Type.Null()]),
	projectedAt: Type.Union([Type.String({ format: "date-time" }), Type.Null()]),
});

const DispatchJobAttemptResponseSchema = Type.Object({
	id: Type.String(),
	attemptNumber: Type.Union([Type.Integer(), Type.Null()]),
	status: Type.Union([Type.String(), Type.Null()]),
	responseCode: Type.Union([Type.Integer(), Type.Null()]),
	responseBody: Type.Union([Type.String(), Type.Null()]),
	errorMessage: Type.Union([Type.String(), Type.Null()]),
	errorType: Type.Union([Type.String(), Type.Null()]),
	durationMillis: Type.Union([Type.Integer(), Type.Null()]),
	attemptedAt: Type.Union([Type.String({ format: "date-time" }), Type.Null()]),
	completedAt: Type.Union([Type.String({ format: "date-time" }), Type.Null()]),
	createdAt: Type.Union([Type.String({ format: "date-time" }), Type.Null()]),
});

const PagedDispatchJobReadListResponseSchema = Type.Object({
	items: Type.Array(DispatchJobReadResponseSchema),
	page: Type.Integer(),
	size: Type.Integer(),
	totalItems: Type.Integer(),
	totalPages: Type.Integer(),
});

const FilterOptionsResponseSchema = Type.Object({
	applications: Type.Array(Type.String()),
	subdomains: Type.Array(Type.String()),
	aggregates: Type.Array(Type.String()),
	codes: Type.Array(Type.String()),
	statuses: Type.Array(Type.String()),
});

const AttemptsResponseSchema = Type.Object({
	dispatchJobId: Type.String(),
	attempts: Type.Array(DispatchJobAttemptResponseSchema),
	total: Type.Integer(),
});

type DispatchJobReadResponse = Static<typeof DispatchJobReadResponseSchema>;
type DispatchJobAttemptResponse = Static<
	typeof DispatchJobAttemptResponseSchema
>;

/**
 * Dependencies for the dispatch jobs API.
 */
export interface DispatchJobsRoutesDeps {
	readonly dispatchJobReadRepository: DispatchJobReadRepository;
}

function toResponse(record: DispatchJobReadRecord): DispatchJobReadResponse {
	return {
		id: record.id,
		externalId: record.externalId,
		source: record.source,
		kind: record.kind,
		code: record.code,
		subject: record.subject,
		eventId: record.eventId,
		correlationId: record.correlationId,
		targetUrl: record.targetUrl,
		protocol: record.protocol,
		application: record.application,
		subdomain: record.subdomain,
		aggregate: record.aggregate,
		clientId: record.clientId,
		subscriptionId: record.subscriptionId,
		serviceAccountId: record.serviceAccountId,
		dispatchPoolId: record.dispatchPoolId,
		messageGroup: record.messageGroup,
		mode: record.mode,
		sequence: record.sequence,
		status: record.status,
		attemptCount: record.attemptCount,
		maxRetries: record.maxRetries,
		lastError: record.lastError,
		timeoutSeconds: record.timeoutSeconds,
		retryStrategy: record.retryStrategy,
		createdAt: record.createdAt.toISOString(),
		updatedAt: record.updatedAt.toISOString(),
		scheduledFor: record.scheduledFor?.toISOString() ?? null,
		expiresAt: record.expiresAt?.toISOString() ?? null,
		completedAt: record.completedAt?.toISOString() ?? null,
		lastAttemptAt: record.lastAttemptAt?.toISOString() ?? null,
		durationMillis: record.durationMillis,
		idempotencyKey: record.idempotencyKey,
		isCompleted: record.isCompleted,
		isTerminal: record.isTerminal,
		projectedAt: record.projectedAt?.toISOString() ?? null,
	};
}

function toAttemptResponse(
	record: DispatchJobAttemptRecord,
): DispatchJobAttemptResponse {
	return {
		id: record.id,
		attemptNumber: record.attemptNumber,
		status: record.status,
		responseCode: record.responseCode,
		responseBody: record.responseBody,
		errorMessage: record.errorMessage,
		errorType: record.errorType,
		durationMillis: record.durationMillis,
		attemptedAt: record.attemptedAt?.toISOString() ?? null,
		completedAt: record.completedAt?.toISOString() ?? null,
		createdAt: record.createdAt?.toISOString() ?? null,
	};
}

/**
 * Parse comma-separated query param into array.
 */
function parseList(value: string | undefined): string[] | undefined {
	if (!value || !value.trim()) return undefined;
	return value
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean);
}

/**
 * Register dispatch job read API routes.
 */
export async function registerDispatchJobsRoutes(
	fastify: FastifyInstance,
	deps: DispatchJobsRoutesDeps,
): Promise<void> {
	const { dispatchJobReadRepository } = deps;

	// GET /api/admin/dispatch-jobs - List dispatch jobs with filters
	fastify.get(
		"/dispatch-jobs",
		{
			preHandler: requirePermission(DISPATCH_JOB_PERMISSIONS.READ),
			schema: {
				querystring: DispatchJobListQuerySchema,
				response: {
					200: PagedDispatchJobReadListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof DispatchJobListQuerySchema>;

			// Merge user-provided clientIds with scope filter
			const accessibleClientIds = getAccessibleClientIds();
			const requestedClientIds = parseList(query.clientIds);
			let scopedClientIds: string[] | undefined;
			if (accessibleClientIds !== null) {
				if (requestedClientIds) {
					scopedClientIds = requestedClientIds.filter((id) =>
						accessibleClientIds.includes(id),
					);
				} else {
					scopedClientIds = accessibleClientIds;
				}
			} else {
				scopedClientIds = requestedClientIds;
			}

			const result = await dispatchJobReadRepository.findPaged(
				{
					clientIds: scopedClientIds,
					statuses: parseList(query.statuses),
					applications: parseList(query.applications),
					subdomains: parseList(query.subdomains),
					aggregates: parseList(query.aggregates),
					codes: parseList(query.codes),
					source: query.source,
					kind: query.kind,
					subscriptionId: query.subscriptionId,
					dispatchPoolId: query.dispatchPoolId,
					messageGroup: query.messageGroup,
					createdAfter: query.createdAfter
						? new Date(query.createdAfter)
						: undefined,
					createdBefore: query.createdBefore
						? new Date(query.createdBefore)
						: undefined,
				},
				{
					page: parseInt(query.page ?? "0", 10) || 0,
					size: parseInt(query.size ?? "20", 10) || 20,
				},
			);

			return jsonSuccess(reply, {
				items: result.items.map(toResponse),
				page: result.page,
				size: result.size,
				totalItems: result.totalItems,
				totalPages: result.totalPages,
			});
		},
	);

	// GET /api/admin/dispatch-jobs/filter-options - Get cascading filter options
	fastify.get(
		"/dispatch-jobs/filter-options",
		{
			preHandler: requirePermission(DISPATCH_JOB_PERMISSIONS.READ),
			schema: {
				querystring: FilterOptionsQuerySchema,
				response: {
					200: FilterOptionsResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof FilterOptionsQuerySchema>;

			// Scope filter options to accessible clients
			const accessibleClientIds = getAccessibleClientIds();
			const requestedClientIds = parseList(query.clientIds);
			let scopedClientIds: string[] | undefined;
			if (accessibleClientIds !== null) {
				if (requestedClientIds) {
					scopedClientIds = requestedClientIds.filter((id) =>
						accessibleClientIds.includes(id),
					);
				} else {
					scopedClientIds = accessibleClientIds;
				}
			} else {
				scopedClientIds = requestedClientIds;
			}

			const options = await dispatchJobReadRepository.getFilterOptions({
				clientIds: scopedClientIds,
				applications: parseList(query.applications),
				subdomains: parseList(query.subdomains),
				aggregates: parseList(query.aggregates),
			});

			return jsonSuccess(reply, options);
		},
	);

	// GET /api/admin/dispatch-jobs/:id - Get single dispatch job
	fastify.get(
		"/dispatch-jobs/:id",
		{
			preHandler: requirePermission(DISPATCH_JOB_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: DispatchJobReadResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const record = await dispatchJobReadRepository.findById(id);

			if (!record || !canAccessResourceByClient(record.clientId)) {
				return notFound(reply, `Dispatch job not found: ${id}`);
			}

			return jsonSuccess(reply, toResponse(record));
		},
	);

	// GET /api/admin/dispatch-jobs/:id/attempts - Get attempts for a dispatch job
	fastify.get(
		"/dispatch-jobs/:id/attempts",
		{
			preHandler: requirePermission(DISPATCH_JOB_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: AttemptsResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;

			// Verify job exists and check access
			const record = await dispatchJobReadRepository.findById(id);
			if (!record || !canAccessResourceByClient(record.clientId)) {
				return notFound(reply, `Dispatch job not found: ${id}`);
			}

			const attempts = await dispatchJobReadRepository.findAttempts(id);

			return jsonSuccess(reply, {
				dispatchJobId: id,
				attempts: attempts.map(toAttemptResponse),
				total: attempts.length,
			});
		},
	);
}
