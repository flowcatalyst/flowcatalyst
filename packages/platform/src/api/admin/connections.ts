/**
 * Connections Admin API
 *
 * REST endpoints for connection management.
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
} from "@flowcatalyst/http";
import { Result } from "@flowcatalyst/application";
import type { UseCase } from "@flowcatalyst/application";

import type {
	CreateConnectionCommand,
	UpdateConnectionCommand,
	DeleteConnectionCommand,
} from "../../application/index.js";
import type {
	ConnectionCreated,
	ConnectionUpdated,
	ConnectionDeleted,
	Connection,
	ConnectionStatus,
} from "../../domain/index.js";
import type {
	ConnectionRepository,
	ConnectionFilters,
} from "../../infrastructure/persistence/index.js";
import {
	requirePermission,
	getAccessibleClientIds,
	canAccessResourceByClient,
} from "../../authorization/index.js";
import { CONNECTION_PERMISSIONS } from "../../authorization/permissions/platform-admin.js";
import type { ConnectionCache } from "../../infrastructure/dispatch/connection-cache.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateConnectionSchema = Type.Object({
	code: Type.String({
		minLength: 2,
		maxLength: 100,
		pattern: "^[a-z][a-z0-9-]*$",
	}),
	name: Type.String({ minLength: 1, maxLength: 255 }),
	description: Type.Optional(
		Type.Union([Type.String({ maxLength: 500 }), Type.Null()]),
	),
	endpoint: Type.String({ minLength: 1, maxLength: 500 }),
	externalId: Type.Optional(
		Type.Union([Type.String({ maxLength: 100 }), Type.Null()]),
	),
	serviceAccountId: Type.String({ minLength: 1 }),
	clientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const UpdateConnectionSchema = Type.Object({
	name: Type.Optional(Type.String({ minLength: 1, maxLength: 255 })),
	description: Type.Optional(
		Type.Union([Type.String({ maxLength: 500 }), Type.Null()]),
	),
	endpoint: Type.Optional(Type.String({ minLength: 1, maxLength: 500 })),
	externalId: Type.Optional(
		Type.Union([Type.String({ maxLength: 100 }), Type.Null()]),
	),
	status: Type.Optional(
		Type.Union([Type.Literal("ACTIVE"), Type.Literal("PAUSED")]),
	),
});

type CreateConnectionBody = Static<typeof CreateConnectionSchema>;
type UpdateConnectionBody = Static<typeof UpdateConnectionSchema>;

// ─── Param Schemas ──────────────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });

// ─── Query Schemas ──────────────────────────────────────────────────────────

const ConnectionListQuerySchema = Type.Object({
	clientId: Type.Optional(Type.String()),
	status: Type.Optional(Type.String()),
	serviceAccountId: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const ConnectionResponseSchema = Type.Object({
	id: Type.String(),
	code: Type.String(),
	name: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	endpoint: Type.String(),
	externalId: Type.Union([Type.String(), Type.Null()]),
	status: Type.String(),
	serviceAccountId: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
	clientIdentifier: Type.Union([Type.String(), Type.Null()]),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const ConnectionListResponseSchema = Type.Object({
	connections: Type.Array(ConnectionResponseSchema),
	total: Type.Integer(),
});

type ConnectionResponse = Static<typeof ConnectionResponseSchema>;

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface ConnectionsRoutesDeps {
	readonly connectionRepository: ConnectionRepository;
	readonly connectionCache?: ConnectionCache | undefined;
	readonly createConnectionUseCase: UseCase<
		CreateConnectionCommand,
		ConnectionCreated
	>;
	readonly updateConnectionUseCase: UseCase<
		UpdateConnectionCommand,
		ConnectionUpdated
	>;
	readonly deleteConnectionUseCase: UseCase<
		DeleteConnectionCommand,
		ConnectionDeleted
	>;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerConnectionsRoutes(
	fastify: FastifyInstance,
	deps: ConnectionsRoutesDeps,
): Promise<void> {
	const {
		connectionRepository,
		connectionCache,
		createConnectionUseCase,
		updateConnectionUseCase,
		deleteConnectionUseCase,
	} = deps;

	// GET /api/admin/connections - List with filters
	fastify.get(
		"/connections",
		{
			preHandler: requirePermission(CONNECTION_PERMISSIONS.READ),
			schema: {
				querystring: ConnectionListQuerySchema,
				response: {
					200: ConnectionListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof ConnectionListQuerySchema>;

			const accessibleClientIds = getAccessibleClientIds();

			const filters: ConnectionFilters = {
				...(query.clientId ? { clientId: query.clientId } : {}),
				...(query.status
					? { status: query.status as ConnectionStatus }
					: {}),
				...(query.serviceAccountId
					? { serviceAccountId: query.serviceAccountId }
					: {}),
				accessibleClientIds,
			};

			const conns = await connectionRepository.findWithFilters(filters);

			return jsonSuccess(reply, {
				connections: conns.map(toConnectionResponse),
				total: conns.length,
			});
		},
	);

	// GET /api/admin/connections/:id - Get by ID
	fastify.get(
		"/connections/:id",
		{
			preHandler: requirePermission(CONNECTION_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: ConnectionResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const connection = await connectionRepository.findById(id);

			if (!connection || !canAccessResourceByClient(connection.clientId)) {
				return notFound(reply, `Connection not found: ${id}`);
			}

			return jsonSuccess(reply, toConnectionResponse(connection));
		},
	);

	// POST /api/admin/connections - Create
	fastify.post(
		"/connections",
		{
			preHandler: requirePermission(CONNECTION_PERMISSIONS.CREATE),
			schema: {
				body: CreateConnectionSchema,
				response: {
					201: ConnectionResponseSchema,
					400: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as CreateConnectionBody;
			const ctx = request.executionContext;

			const command: CreateConnectionCommand = {
				code: body.code,
				name: body.name,
				description: body.description ?? null,
				endpoint: body.endpoint,
				externalId: body.externalId ?? null,
				serviceAccountId: body.serviceAccountId,
				clientId: body.clientId ?? null,
			};

			const result = await createConnectionUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const conn = await connectionRepository.findById(
					result.value.getData().connectionId,
				);
				if (conn) {
					connectionCache?.set(conn);
					return jsonCreated(reply, toConnectionResponse(conn));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /api/admin/connections/:id - Update
	fastify.put(
		"/connections/:id",
		{
			preHandler: requirePermission(CONNECTION_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				body: UpdateConnectionSchema,
				response: {
					200: ConnectionResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const body = request.body as UpdateConnectionBody;
			const ctx = request.executionContext;

			const command: UpdateConnectionCommand = {
				connectionId: id,
				...(body.name !== undefined ? { name: body.name } : {}),
				...(body.description !== undefined
					? { description: body.description }
					: {}),
				...(body.endpoint !== undefined
					? { endpoint: body.endpoint }
					: {}),
				...(body.externalId !== undefined
					? { externalId: body.externalId }
					: {}),
				...(body.status !== undefined
					? { status: body.status as ConnectionStatus }
					: {}),
			};

			const result = await updateConnectionUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const conn = await connectionRepository.findById(id);
				if (conn) {
					connectionCache?.set(conn);
					return jsonSuccess(reply, toConnectionResponse(conn));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/connections/:id - Delete
	fastify.delete(
		"/connections/:id",
		{
			preHandler: requirePermission(CONNECTION_PERMISSIONS.DELETE),
			schema: {
				params: IdParam,
				response: {
					204: Type.Null(),
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const ctx = request.executionContext;

			const command: DeleteConnectionCommand = { connectionId: id };
			const result = await deleteConnectionUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				connectionCache?.remove(id);
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/connections/:id/pause - Pause
	fastify.post(
		"/connections/:id/pause",
		{
			preHandler: requirePermission(CONNECTION_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				response: {
					200: ConnectionResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const ctx = request.executionContext;

			const command: UpdateConnectionCommand = {
				connectionId: id,
				status: "PAUSED",
			};
			const result = await updateConnectionUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const conn = await connectionRepository.findById(id);
				if (conn) {
					connectionCache?.set(conn);
					return jsonSuccess(reply, toConnectionResponse(conn));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/connections/:id/activate - Activate
	fastify.post(
		"/connections/:id/activate",
		{
			preHandler: requirePermission(CONNECTION_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				response: {
					200: ConnectionResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const ctx = request.executionContext;

			const command: UpdateConnectionCommand = {
				connectionId: id,
				status: "ACTIVE",
			};
			const result = await updateConnectionUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const conn = await connectionRepository.findById(id);
				if (conn) {
					connectionCache?.set(conn);
					return jsonSuccess(reply, toConnectionResponse(conn));
				}
			}

			return sendResult(reply, result);
		},
	);
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toConnectionResponse(conn: Connection): ConnectionResponse {
	return {
		id: conn.id,
		code: conn.code,
		name: conn.name,
		description: conn.description,
		endpoint: conn.endpoint,
		externalId: conn.externalId,
		status: conn.status,
		serviceAccountId: conn.serviceAccountId,
		clientId: conn.clientId,
		clientIdentifier: conn.clientIdentifier,
		createdAt: conn.createdAt.toISOString(),
		updatedAt: conn.updatedAt.toISOString(),
	};
}
