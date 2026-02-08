/**
 * Raw Dispatch Jobs Debug BFF API
 *
 * Admin/debug endpoint for querying raw dispatch jobs from the transactional
 * dispatch_jobs table. The raw table has minimal indexes optimized for writes.
 * Regular UI queries should use /api/admin/dispatch-jobs which queries the
 * read-optimized projection.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import { jsonSuccess, notFound, ErrorResponseSchema } from '@flowcatalyst/http';
import { desc, sql, eq } from 'drizzle-orm';
import { dispatchJobs, type DispatchJobRecord } from '@flowcatalyst/persistence';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import { requirePermission } from '../../authorization/index.js';
import { DISPATCH_JOB_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const PaginationQuery = Type.Object({
	page: Type.Optional(Type.Integer({ minimum: 0, default: 0 })),
	size: Type.Optional(Type.Integer({ minimum: 1, maximum: 100, default: 20 })),
});

const IdParam = Type.Object({ id: Type.String() });

// ─── Response Schemas ───────────────────────────────────────────────────────

const RawDispatchJobResponseSchema = Type.Object({
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
	clientId: Type.Union([Type.String(), Type.Null()]),
	subscriptionId: Type.Union([Type.String(), Type.Null()]),
	serviceAccountId: Type.Union([Type.String(), Type.Null()]),
	dispatchPoolId: Type.Union([Type.String(), Type.Null()]),
	messageGroup: Type.Union([Type.String(), Type.Null()]),
	mode: Type.String(),
	sequence: Type.Integer(),
	status: Type.String(),
	attemptCount: Type.Integer(),
	maxRetries: Type.Integer(),
	lastError: Type.Union([Type.String(), Type.Null()]),
	timeoutSeconds: Type.Integer(),
	retryStrategy: Type.Union([Type.String(), Type.Null()]),
	idempotencyKey: Type.Union([Type.String(), Type.Null()]),
	createdAt: Type.String({ format: 'date-time' }),
	updatedAt: Type.String({ format: 'date-time' }),
	scheduledFor: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
	completedAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
	payloadContentType: Type.Union([Type.String(), Type.Null()]),
	payloadLength: Type.Integer(),
});

const PagedRawDispatchJobResponseSchema = Type.Object({
	items: Type.Array(RawDispatchJobResponseSchema),
	page: Type.Integer(),
	size: Type.Integer(),
	totalItems: Type.Integer(),
	totalPages: Type.Integer(),
});

/**
 * Dependencies for debug dispatch jobs BFF routes.
 */
export interface DebugDispatchJobsBffDeps {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	readonly db: PostgresJsDatabase<any>;
}

/**
 * Register debug dispatch jobs BFF routes.
 */
export async function registerDebugDispatchJobsBffRoutes(
	fastify: FastifyInstance,
	deps: DebugDispatchJobsBffDeps,
): Promise<void> {
	const { db } = deps;

	// GET /api/bff/debug/dispatch-jobs - List raw dispatch jobs
	fastify.get(
		'/dispatch-jobs',
		{
			preHandler: requirePermission(DISPATCH_JOB_PERMISSIONS.VIEW_RAW),
			schema: {
				querystring: PaginationQuery,
				response: {
					200: PagedRawDispatchJobResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof PaginationQuery>;
			const page = query.page ?? 0;
			const size = query.size ?? 20;
			const offset = page * size;

			const [countResult] = await db
				.select({ count: sql<number>`count(*)` })
				.from(dispatchJobs);
			const totalItems = Number(countResult?.count ?? 0);
			const totalPages = Math.ceil(totalItems / size);

			const records = await db
				.select()
				.from(dispatchJobs)
				.orderBy(desc(dispatchJobs.createdAt))
				.limit(size)
				.offset(offset);

			return jsonSuccess(reply, {
				items: records.map(toRawDispatchJobResponse),
				page,
				size,
				totalItems,
				totalPages,
			});
		},
	);

	// GET /api/bff/debug/dispatch-jobs/:id - Get single raw dispatch job
	fastify.get(
		'/dispatch-jobs/:id',
		{
			preHandler: requirePermission(DISPATCH_JOB_PERMISSIONS.VIEW_RAW),
			schema: {
				params: IdParam,
				response: {
					200: RawDispatchJobResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;

			const [record] = await db
				.select()
				.from(dispatchJobs)
				.where(eq(dispatchJobs.id, id))
				.limit(1);

			if (!record) {
				return notFound(reply, `Raw dispatch job not found: ${id}`);
			}

			return jsonSuccess(reply, toRawDispatchJobResponse(record));
		},
	);
}

function toRawDispatchJobResponse(record: DispatchJobRecord) {
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
		idempotencyKey: record.idempotencyKey,
		createdAt: record.createdAt.toISOString(),
		updatedAt: record.updatedAt.toISOString(),
		scheduledFor: record.scheduledFor?.toISOString() ?? null,
		completedAt: record.completedAt?.toISOString() ?? null,
		payloadContentType: record.payloadContentType,
		payloadLength: record.payload?.length ?? 0,
	};
}
