/**
 * Event Types Admin API
 *
 * REST endpoints for event type management.
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import {
	sendResult,
	jsonCreated,
	jsonSuccess,
	noContent,
	notFound,
	ErrorResponseSchema,
	SyncResponseSchema,
} from "@flowcatalyst/http";
import { Result } from "@flowcatalyst/application";
import type { UseCase } from "@flowcatalyst/application";

import type {
	CreateEventTypeCommand,
	UpdateEventTypeCommand,
	DeleteEventTypeCommand,
	ArchiveEventTypeCommand,
	AddSchemaCommand,
	FinaliseSchemaCommand,
	DeprecateSchemaCommand,
	SyncEventTypesCommand,
} from "../../application/index.js";
import type {
	EventTypeCreated,
	EventTypeUpdated,
	EventTypeDeleted,
	EventTypeArchived,
	SchemaAdded,
	SchemaFinalised,
	SchemaDeprecated,
	EventTypesSynced,
	EventType,
	SpecVersion,
	SchemaType,
	EventTypeStatus,
} from "../../domain/index.js";
import { parseCodeSegments } from "../../domain/index.js";
import type {
	EventTypeRepository,
	EventTypeFilters,
} from "../../infrastructure/persistence/index.js";
import { requirePermission } from "../../authorization/index.js";
import { EVENT_TYPE_PERMISSIONS } from "../../authorization/permissions/platform-admin.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateEventTypeSchema = Type.Object({
	application: Type.String({ minLength: 1, maxLength: 50 }),
	subdomain: Type.String({ minLength: 1, maxLength: 50 }),
	aggregate: Type.String({ minLength: 1, maxLength: 50 }),
	event: Type.String({ minLength: 1, maxLength: 50 }),
	name: Type.String({ minLength: 1, maxLength: 100 }),
	description: Type.Optional(
		Type.Union([Type.String({ maxLength: 255 }), Type.Null()]),
	),
	clientScoped: Type.Optional(Type.Boolean()),
});

const UpdateEventTypeSchema = Type.Object({
	name: Type.Optional(Type.String({ minLength: 1, maxLength: 100 })),
	description: Type.Optional(
		Type.Union([Type.String({ maxLength: 255 }), Type.Null()]),
	),
});

const AddSchemaSchema = Type.Object({
	version: Type.String({ pattern: "^\\d+\\.\\d+$" }),
	mimeType: Type.String({ minLength: 1, maxLength: 100 }),
	schemaContent: Type.Unknown(),
	schemaType: Type.Union([
		Type.Literal("JSON_SCHEMA"),
		Type.Literal("PROTO"),
		Type.Literal("XSD"),
	]),
});

const SyncEventTypesSchema = Type.Object({
	applicationCode: Type.String({ minLength: 1, maxLength: 50 }),
	eventTypes: Type.Array(
		Type.Object({
			subdomain: Type.String({ minLength: 1 }),
			aggregate: Type.String({ minLength: 1 }),
			event: Type.String({ minLength: 1 }),
			name: Type.String({ minLength: 1 }),
			description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
			clientScoped: Type.Optional(Type.Boolean()),
		}),
	),
	removeUnlisted: Type.Optional(Type.Boolean()),
});

type CreateEventTypeBody = Static<typeof CreateEventTypeSchema>;
type SyncEventTypesBody = Static<typeof SyncEventTypesSchema>;

// ─── Param Schemas ──────────────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });
const IdVersionParam = Type.Object({
	id: Type.String(),
	version: Type.String(),
});

// ─── Query Schemas ──────────────────────────────────────────────────────────

const EventTypeListQuerySchema = Type.Object({
	status: Type.Optional(Type.String()),
	application: Type.Optional(
		Type.Union([Type.String(), Type.Array(Type.String())]),
	),
	subdomain: Type.Optional(
		Type.Union([Type.String(), Type.Array(Type.String())]),
	),
	aggregate: Type.Optional(
		Type.Union([Type.String(), Type.Array(Type.String())]),
	),
});

const SubdomainFilterQuerySchema = Type.Object({
	application: Type.Optional(
		Type.Union([Type.String(), Type.Array(Type.String())]),
	),
});

const AggregateFilterQuerySchema = Type.Object({
	application: Type.Optional(
		Type.Union([Type.String(), Type.Array(Type.String())]),
	),
	subdomain: Type.Optional(
		Type.Union([Type.String(), Type.Array(Type.String())]),
	),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const SpecVersionResponseSchema = Type.Object({
	id: Type.String(),
	version: Type.String(),
	mimeType: Type.String(),
	schemaContent: Type.Union([Type.Unknown(), Type.Null()]),
	schemaType: Type.String(),
	status: Type.String(),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const EventTypeResponseSchema = Type.Object({
	id: Type.String(),
	code: Type.String(),
	name: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	application: Type.Union([Type.String(), Type.Null()]),
	subdomain: Type.Union([Type.String(), Type.Null()]),
	aggregate: Type.Union([Type.String(), Type.Null()]),
	event: Type.Union([Type.String(), Type.Null()]),
	specVersions: Type.Array(SpecVersionResponseSchema),
	status: Type.String(),
	source: Type.String(),
	clientScoped: Type.Boolean(),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const EventTypesListResponseSchema = Type.Object({
	eventTypes: Type.Array(EventTypeResponseSchema),
	total: Type.Integer(),
});

const FilterValuesResponseSchema = Type.Object({
	values: Type.Array(Type.String()),
});

type EventTypeResponse = Static<typeof EventTypeResponseSchema>;
type SpecVersionResponse = Static<typeof SpecVersionResponseSchema>;

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface EventTypesRoutesDeps {
	readonly eventTypeRepository: EventTypeRepository;
	readonly createEventTypeUseCase: UseCase<
		CreateEventTypeCommand,
		EventTypeCreated
	>;
	readonly updateEventTypeUseCase: UseCase<
		UpdateEventTypeCommand,
		EventTypeUpdated
	>;
	readonly deleteEventTypeUseCase: UseCase<
		DeleteEventTypeCommand,
		EventTypeDeleted
	>;
	readonly archiveEventTypeUseCase: UseCase<
		ArchiveEventTypeCommand,
		EventTypeArchived
	>;
	readonly addSchemaUseCase: UseCase<AddSchemaCommand, SchemaAdded>;
	readonly finaliseSchemaUseCase: UseCase<
		FinaliseSchemaCommand,
		SchemaFinalised
	>;
	readonly deprecateSchemaUseCase: UseCase<
		DeprecateSchemaCommand,
		SchemaDeprecated
	>;
	readonly syncEventTypesUseCase: UseCase<
		SyncEventTypesCommand,
		EventTypesSynced
	>;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerEventTypesRoutes(
	fastify: FastifyInstance,
	deps: EventTypesRoutesDeps,
): Promise<void> {
	const {
		eventTypeRepository,
		createEventTypeUseCase,
		updateEventTypeUseCase,
		deleteEventTypeUseCase,
		archiveEventTypeUseCase,
		addSchemaUseCase,
		finaliseSchemaUseCase,
		deprecateSchemaUseCase,
		syncEventTypesUseCase,
	} = deps;

	// GET /api/admin/event-types - List with filters
	fastify.get(
		"/event-types",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
			schema: {
				querystring: EventTypeListQuerySchema,
				response: {
					200: EventTypesListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof EventTypeListQuerySchema>;

			const applications = toArray(query.application);
			const subdomains = toArray(query.subdomain);
			const aggregates = toArray(query.aggregate);

			const filters: EventTypeFilters = {
				...(query.status ? { status: query.status as EventTypeStatus } : {}),
				...(applications.length > 0 ? { applications } : {}),
				...(subdomains.length > 0 ? { subdomains } : {}),
				...(aggregates.length > 0 ? { aggregates } : {}),
			};

			const eventTypes = await eventTypeRepository.findWithFilters(filters);

			return jsonSuccess(reply, {
				eventTypes: eventTypes.map(toEventTypeResponse),
				total: eventTypes.length,
			});
		},
	);

	// GET /api/admin/event-types/:id - Get by ID
	fastify.get(
		"/event-types/:id",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: EventTypeResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const eventType = await eventTypeRepository.findById(id);

			if (!eventType) {
				return notFound(reply, `Event type not found: ${id}`);
			}

			return jsonSuccess(reply, toEventTypeResponse(eventType));
		},
	);

	// POST /api/admin/event-types - Create
	fastify.post(
		"/event-types",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.CREATE),
			schema: {
				body: CreateEventTypeSchema,
				response: {
					201: EventTypeResponseSchema,
					400: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as CreateEventTypeBody;
			const ctx = request.executionContext;

			const command: CreateEventTypeCommand = {
				application: body.application,
				subdomain: body.subdomain,
				aggregate: body.aggregate,
				event: body.event,
				name: body.name,
				description: body.description ?? null,
				clientScoped: body.clientScoped ?? false,
			};

			const result = await createEventTypeUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const eventType = await eventTypeRepository.findById(
					result.value.getData().eventTypeId,
				);
				if (eventType) {
					return jsonCreated(reply, toEventTypeResponse(eventType));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PATCH /api/admin/event-types/:id - Update metadata
	fastify.patch(
		"/event-types/:id",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				body: UpdateEventTypeSchema,
				response: {
					200: EventTypeResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const body = request.body as Static<typeof UpdateEventTypeSchema>;
			const ctx = request.executionContext;

			const command: UpdateEventTypeCommand = {
				eventTypeId: id,
				...(body.name !== undefined ? { name: body.name } : {}),
				...(body.description !== undefined
					? { description: body.description }
					: {}),
			};

			const result = await updateEventTypeUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const eventType = await eventTypeRepository.findById(id);
				if (eventType) {
					return jsonSuccess(reply, toEventTypeResponse(eventType));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/event-types/:id - Delete
	fastify.delete(
		"/event-types/:id",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.DELETE),
			schema: {
				params: IdParam,
				response: {
					204: Type.Null(),
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const ctx = request.executionContext;

			const command: DeleteEventTypeCommand = { eventTypeId: id };
			const result = await deleteEventTypeUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/event-types/:id/archive - Archive
	fastify.post(
		"/event-types/:id/archive",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.ARCHIVE),
			schema: {
				params: IdParam,
				response: {
					200: EventTypeResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const ctx = request.executionContext;

			const command: ArchiveEventTypeCommand = { eventTypeId: id };
			const result = await archiveEventTypeUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const eventType = await eventTypeRepository.findById(id);
				if (eventType) {
					return jsonSuccess(reply, toEventTypeResponse(eventType));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/event-types/:id/schemas - Add schema
	fastify.post(
		"/event-types/:id/schemas",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.MANAGE_SCHEMA),
			schema: {
				params: IdParam,
				body: AddSchemaSchema,
				response: {
					201: EventTypeResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const body = request.body as Static<typeof AddSchemaSchema>;
			const ctx = request.executionContext;

			const command: AddSchemaCommand = {
				eventTypeId: id,
				version: body.version,
				mimeType: body.mimeType,
				schemaContent: body.schemaContent,
				schemaType: body.schemaType as SchemaType,
			};

			const result = await addSchemaUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const eventType = await eventTypeRepository.findById(id);
				if (eventType) {
					return jsonCreated(reply, toEventTypeResponse(eventType));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/event-types/:id/schemas/:version/finalise - Finalise schema
	fastify.post(
		"/event-types/:id/schemas/:version/finalise",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.MANAGE_SCHEMA),
			schema: {
				params: IdVersionParam,
				response: {
					200: EventTypeResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id, version } = request.params as Static<typeof IdVersionParam>;
			const ctx = request.executionContext;

			const command: FinaliseSchemaCommand = {
				eventTypeId: id,
				version,
			};

			const result = await finaliseSchemaUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const eventType = await eventTypeRepository.findById(id);
				if (eventType) {
					return jsonSuccess(reply, toEventTypeResponse(eventType));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/event-types/:id/schemas/:version/deprecate - Deprecate schema
	fastify.post(
		"/event-types/:id/schemas/:version/deprecate",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.MANAGE_SCHEMA),
			schema: {
				params: IdVersionParam,
				response: {
					200: EventTypeResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id, version } = request.params as Static<typeof IdVersionParam>;
			const ctx = request.executionContext;

			const command: DeprecateSchemaCommand = {
				eventTypeId: id,
				version,
			};

			const result = await deprecateSchemaUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const eventType = await eventTypeRepository.findById(id);
				if (eventType) {
					return jsonSuccess(reply, toEventTypeResponse(eventType));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/event-types/sync - Sync event types from SDK
	fastify.post(
		"/event-types/sync",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.SYNC),
			schema: {
				body: SyncEventTypesSchema,
				response: {
					200: SyncResponseSchema,
					400: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as SyncEventTypesBody;
			const ctx = request.executionContext;

			const command: SyncEventTypesCommand = {
				applicationCode: body.applicationCode,
				eventTypes: body.eventTypes,
				removeUnlisted: body.removeUnlisted ?? false,
			};

			const result = await syncEventTypesUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const data = result.value.getData();
				return jsonSuccess(reply, {
					applicationCode: data.applicationCode,
					created: data.eventTypesCreated,
					updated: data.eventTypesUpdated,
					deleted: data.eventTypesDeleted,
					syncedCodes: data.syncedEventTypeCodes,
				});
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/event-types/filters/applications - Distinct applications
	fastify.get(
		"/event-types/filters/applications",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
			schema: {
				response: {
					200: FilterValuesResponseSchema,
				},
			},
		},
		async (_request, reply) => {
			const applications = await eventTypeRepository.findDistinctApplications();
			return jsonSuccess(reply, { values: applications });
		},
	);

	// GET /api/admin/event-types/filters/subdomains - Distinct subdomains
	fastify.get(
		"/event-types/filters/subdomains",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
			schema: {
				querystring: SubdomainFilterQuerySchema,
				response: {
					200: FilterValuesResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof SubdomainFilterQuerySchema>;
			const applications = toArray(query.application);
			const subdomains = await eventTypeRepository.findDistinctSubdomains(
				applications.length > 0 ? applications : undefined,
			);
			return jsonSuccess(reply, { values: subdomains });
		},
	);

	// GET /api/admin/event-types/filters/aggregates - Distinct aggregates
	fastify.get(
		"/event-types/filters/aggregates",
		{
			preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
			schema: {
				querystring: AggregateFilterQuerySchema,
				response: {
					200: FilterValuesResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof AggregateFilterQuerySchema>;
			const applications = toArray(query.application);
			const subdomains = toArray(query.subdomain);
			const aggregates = await eventTypeRepository.findDistinctAggregates(
				applications.length > 0 ? applications : undefined,
				subdomains.length > 0 ? subdomains : undefined,
			);
			return jsonSuccess(reply, { values: aggregates });
		},
	);
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toArray(value: string | string[] | undefined): string[] {
	if (!value) return [];
	return Array.isArray(value) ? value : [value];
}

function toEventTypeResponse(eventType: EventType): EventTypeResponse {
	const segments = parseCodeSegments(eventType.code);
	return {
		id: eventType.id,
		code: eventType.code,
		name: eventType.name,
		description: eventType.description,
		application: segments?.application ?? null,
		subdomain: segments?.subdomain ?? null,
		aggregate: segments?.aggregate ?? null,
		event: segments?.event ?? null,
		specVersions: eventType.specVersions.map(toSpecVersionResponse),
		status: eventType.status,
		source: eventType.source,
		clientScoped: eventType.clientScoped,
		createdAt: eventType.createdAt.toISOString(),
		updatedAt: eventType.updatedAt.toISOString(),
	};
}

function toSpecVersionResponse(sv: SpecVersion): SpecVersionResponse {
	return {
		id: sv.id,
		version: sv.version,
		mimeType: sv.mimeType,
		schemaContent: sv.schemaContent,
		schemaType: sv.schemaType,
		status: sv.status,
		createdAt: sv.createdAt.toISOString(),
		updatedAt: sv.updatedAt.toISOString(),
	};
}
