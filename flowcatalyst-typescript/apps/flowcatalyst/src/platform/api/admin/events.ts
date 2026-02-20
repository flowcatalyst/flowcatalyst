/**
 * Events BFF API
 *
 * Read-only endpoints for browsing events from the events_read projection.
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import { jsonSuccess, notFound, ErrorResponseSchema } from "@flowcatalyst/http";
import type { EventReadRecord } from "@flowcatalyst/persistence";

import type { EventReadRepository } from "../../infrastructure/persistence/index.js";
import {
	requirePermission,
	getAccessibleClientIds,
	canAccessResourceByClient,
} from "../../authorization/index.js";
import { EVENT_PERMISSIONS } from "../../authorization/permissions/platform-admin.js";

// ─── Param Schemas ──────────────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });

// ─── Query Schemas ──────────────────────────────────────────────────────────

const EventListQuerySchema = Type.Object({
	clientIds: Type.Optional(Type.String()),
	applications: Type.Optional(Type.String()),
	subdomains: Type.Optional(Type.String()),
	aggregates: Type.Optional(Type.String()),
	types: Type.Optional(Type.String()),
	source: Type.Optional(Type.String()),
	subject: Type.Optional(Type.String()),
	correlationId: Type.Optional(Type.String()),
	messageGroup: Type.Optional(Type.String()),
	timeAfter: Type.Optional(Type.String()),
	timeBefore: Type.Optional(Type.String()),
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

const EventReadResponseSchema = Type.Object({
	id: Type.String(),
	specVersion: Type.Union([Type.String(), Type.Null()]),
	type: Type.String(),
	application: Type.Union([Type.String(), Type.Null()]),
	subdomain: Type.Union([Type.String(), Type.Null()]),
	aggregate: Type.Union([Type.String(), Type.Null()]),
	source: Type.String(),
	subject: Type.Union([Type.String(), Type.Null()]),
	time: Type.String({ format: "date-time" }),
	data: Type.Union([Type.String(), Type.Null()]),
	messageGroup: Type.Union([Type.String(), Type.Null()]),
	correlationId: Type.Union([Type.String(), Type.Null()]),
	causationId: Type.Union([Type.String(), Type.Null()]),
	deduplicationId: Type.Union([Type.String(), Type.Null()]),
	clientId: Type.Union([Type.String(), Type.Null()]),
	projectedAt: Type.String({ format: "date-time" }),
});

const PagedEventReadListResponseSchema = Type.Object({
	items: Type.Array(EventReadResponseSchema),
	page: Type.Integer(),
	size: Type.Integer(),
	totalItems: Type.Integer(),
	totalPages: Type.Integer(),
});

const FilterOptionsResponseSchema = Type.Object({
	applications: Type.Array(Type.String()),
	subdomains: Type.Array(Type.String()),
	aggregates: Type.Array(Type.String()),
	types: Type.Array(Type.String()),
});

type EventReadResponse = Static<typeof EventReadResponseSchema>;

/**
 * Dependencies for the events API.
 */
export interface EventsRoutesDeps {
	readonly eventReadRepository: EventReadRepository;
}

function toResponse(record: EventReadRecord): EventReadResponse {
	return {
		id: record.id,
		specVersion: record.specVersion,
		type: record.type,
		application: record.application,
		subdomain: record.subdomain,
		aggregate: record.aggregate,
		source: record.source,
		subject: record.subject,
		time: record.time.toISOString(),
		data: record.data,
		messageGroup: record.messageGroup,
		correlationId: record.correlationId,
		causationId: record.causationId,
		deduplicationId: record.deduplicationId,
		clientId: record.clientId,
		projectedAt: record.projectedAt.toISOString(),
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
 * Register event read API routes.
 */
export async function registerEventsRoutes(
	fastify: FastifyInstance,
	deps: EventsRoutesDeps,
): Promise<void> {
	const { eventReadRepository } = deps;

	// GET /api/admin/events - List events with filters
	fastify.get(
		"/events",
		{
			preHandler: requirePermission(EVENT_PERMISSIONS.READ),
			schema: {
				querystring: EventListQuerySchema,
				response: {
					200: PagedEventReadListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof EventListQuerySchema>;

			// Merge user-provided clientIds with scope filter
			const accessibleClientIds = getAccessibleClientIds();
			const requestedClientIds = parseList(query.clientIds);
			let scopedClientIds: string[] | undefined;
			if (accessibleClientIds !== null) {
				// Restricted scope: intersect with any user-requested filter
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

			const result = await eventReadRepository.findPaged(
				{
					clientIds: scopedClientIds,
					applications: parseList(query.applications),
					subdomains: parseList(query.subdomains),
					aggregates: parseList(query.aggregates),
					types: parseList(query.types),
					source: query.source,
					subject: query.subject,
					correlationId: query.correlationId,
					messageGroup: query.messageGroup,
					timeAfter: query.timeAfter ? new Date(query.timeAfter) : undefined,
					timeBefore: query.timeBefore ? new Date(query.timeBefore) : undefined,
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

	// GET /api/admin/events/filter-options - Get cascading filter options
	fastify.get(
		"/events/filter-options",
		{
			preHandler: requirePermission(EVENT_PERMISSIONS.READ),
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

			const options = await eventReadRepository.getFilterOptions({
				clientIds: scopedClientIds,
				applications: parseList(query.applications),
				subdomains: parseList(query.subdomains),
				aggregates: parseList(query.aggregates),
			});

			return jsonSuccess(reply, options);
		},
	);

	// GET /api/admin/events/:id - Get single event
	fastify.get(
		"/events/:id",
		{
			preHandler: requirePermission(EVENT_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: EventReadResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const record = await eventReadRepository.findById(id);

			if (!record || !canAccessResourceByClient(record.clientId)) {
				return notFound(reply, `Event not found: ${id}`);
			}

			return jsonSuccess(reply, toResponse(record));
		},
	);
}
