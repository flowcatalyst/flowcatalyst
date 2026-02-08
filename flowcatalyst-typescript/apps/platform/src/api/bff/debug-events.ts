/**
 * Raw Events Debug BFF API
 *
 * Admin/debug endpoint for querying raw events from the transactional
 * events table. The raw table has minimal indexes optimized for writes.
 * Regular UI queries should use /api/admin/events which queries the
 * read-optimized projection.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import { jsonSuccess, notFound, ErrorResponseSchema } from '@flowcatalyst/http';
import { desc, sql, eq } from 'drizzle-orm';
import { events, type Event } from '@flowcatalyst/persistence';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import { requirePermission } from '../../authorization/index.js';
import { EVENT_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const PaginationQuery = Type.Object({
	page: Type.Optional(Type.Integer({ minimum: 0, default: 0 })),
	size: Type.Optional(Type.Integer({ minimum: 1, maximum: 100, default: 20 })),
});

const IdParam = Type.Object({ id: Type.String() });

// ─── Response Schemas ───────────────────────────────────────────────────────

const ContextDataSchema = Type.Object({
	key: Type.String(),
	value: Type.String(),
});

const RawEventResponseSchema = Type.Object({
	id: Type.String(),
	specVersion: Type.String(),
	type: Type.String(),
	source: Type.String(),
	subject: Type.Union([Type.String(), Type.Null()]),
	time: Type.String({ format: 'date-time' }),
	data: Type.Unknown(),
	messageGroup: Type.Union([Type.String(), Type.Null()]),
	correlationId: Type.Union([Type.String(), Type.Null()]),
	causationId: Type.Union([Type.String(), Type.Null()]),
	deduplicationId: Type.Union([Type.String(), Type.Null()]),
	contextData: Type.Union([Type.Array(ContextDataSchema), Type.Null()]),
	clientId: Type.Union([Type.String(), Type.Null()]),
});

const PagedRawEventResponseSchema = Type.Object({
	items: Type.Array(RawEventResponseSchema),
	page: Type.Integer(),
	size: Type.Integer(),
	totalItems: Type.Integer(),
	totalPages: Type.Integer(),
});

/**
 * Dependencies for debug events BFF routes.
 */
export interface DebugEventsBffDeps {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	readonly db: PostgresJsDatabase<any>;
}

/**
 * Register debug events BFF routes.
 */
export async function registerDebugEventsBffRoutes(
	fastify: FastifyInstance,
	deps: DebugEventsBffDeps,
): Promise<void> {
	const { db } = deps;

	// GET /api/bff/debug/events - List raw events
	fastify.get(
		'/events',
		{
			preHandler: requirePermission(EVENT_PERMISSIONS.VIEW_RAW),
			schema: {
				querystring: PaginationQuery,
				response: {
					200: PagedRawEventResponseSchema,
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
				.from(events);
			const totalItems = Number(countResult?.count ?? 0);
			const totalPages = Math.ceil(totalItems / size);

			const records = await db
				.select()
				.from(events)
				.orderBy(desc(events.createdAt))
				.limit(size)
				.offset(offset);

			return jsonSuccess(reply, {
				items: records.map(toRawEventResponse),
				page,
				size,
				totalItems,
				totalPages,
			});
		},
	);

	// GET /api/bff/debug/events/:id - Get single raw event
	fastify.get(
		'/events/:id',
		{
			preHandler: requirePermission(EVENT_PERMISSIONS.VIEW_RAW),
			schema: {
				params: IdParam,
				response: {
					200: RawEventResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;

			const [record] = await db
				.select()
				.from(events)
				.where(eq(events.id, id))
				.limit(1);

			if (!record) {
				return notFound(reply, `Raw event not found: ${id}`);
			}

			return jsonSuccess(reply, toRawEventResponse(record));
		},
	);
}

function toRawEventResponse(record: Event) {
	return {
		id: record.id,
		specVersion: record.specVersion,
		type: record.type,
		source: record.source,
		subject: record.subject,
		time: record.time.toISOString(),
		data: record.data,
		messageGroup: record.messageGroup,
		correlationId: record.correlationId,
		causationId: record.causationId,
		deduplicationId: record.deduplicationId,
		contextData: record.contextData ?? null,
		clientId: record.clientId,
	};
}
