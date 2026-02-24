/**
 * CORS Admin API
 *
 * REST endpoints for CORS allowed origin management.
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
	AddCorsOriginCommand,
	DeleteCorsOriginCommand,
} from "../../application/index.js";
import type { CorsOriginAdded, CorsOriginDeleted } from "../../domain/index.js";
import type { CorsAllowedOriginRepository } from "../../infrastructure/persistence/index.js";
import { requirePermission } from "../../authorization/index.js";
import { CORS_ORIGIN_PERMISSIONS } from "../../authorization/permissions/platform-admin.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateCorsOriginSchema = Type.Object({
	origin: Type.String({ minLength: 1, maxLength: 500 }),
	description: Type.Optional(Type.String({ maxLength: 1000 })),
});

const IdParam = Type.Object({ id: Type.String() });

// ─── Response Schemas ───────────────────────────────────────────────────────

const CorsOriginResponseSchema = Type.Object({
	id: Type.String(),
	origin: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	createdBy: Type.Union([Type.String(), Type.Null()]),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const CorsOriginsListResponseSchema = Type.Object({
	corsOrigins: Type.Array(CorsOriginResponseSchema),
	total: Type.Integer(),
});

const AllowedOriginsResponseSchema = Type.Object({
	origins: Type.Array(Type.String()),
});

type CorsOriginResponse = Static<typeof CorsOriginResponseSchema>;

/**
 * Dependencies for the CORS admin API.
 */
export interface CorsRoutesDeps {
	readonly corsAllowedOriginRepository: CorsAllowedOriginRepository;
	readonly addCorsOriginUseCase: UseCase<AddCorsOriginCommand, CorsOriginAdded>;
	readonly deleteCorsOriginUseCase: UseCase<
		DeleteCorsOriginCommand,
		CorsOriginDeleted
	>;
}

/**
 * Register CORS admin API routes.
 */
export async function registerCorsRoutes(
	fastify: FastifyInstance,
	deps: CorsRoutesDeps,
): Promise<void> {
	const {
		corsAllowedOriginRepository,
		addCorsOriginUseCase,
		deleteCorsOriginUseCase,
	} = deps;

	// POST /api/admin/platform/cors - Add CORS origin
	fastify.post(
		"/platform/cors",
		{
			preHandler: requirePermission(CORS_ORIGIN_PERMISSIONS.CREATE),
			schema: {
				body: CreateCorsOriginSchema,
				response: {
					201: CorsOriginResponseSchema,
					400: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as Static<typeof CreateCorsOriginSchema>;
			const ctx = request.executionContext;

			const command: AddCorsOriginCommand = {
				origin: body.origin,
				description: body.description ?? null,
			};

			const result = await addCorsOriginUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const origin = await corsAllowedOriginRepository.findById(
					result.value.getData().originId,
				);
				if (origin) {
					return jsonCreated(reply, toCorsOriginResponse(origin));
				}
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/platform/cors - List CORS origins
	fastify.get(
		"/platform/cors",
		{
			preHandler: requirePermission(CORS_ORIGIN_PERMISSIONS.READ),
			schema: {
				response: {
					200: CorsOriginsListResponseSchema,
				},
			},
		},
		async (_request, reply) => {
			const corsOrigins = await corsAllowedOriginRepository.findAll();
			const total = await corsAllowedOriginRepository.count();

			return jsonSuccess(reply, {
				corsOrigins: corsOrigins.map(toCorsOriginResponse),
				total,
			});
		},
	);

	// GET /api/admin/platform/cors/allowed - Get allowed origins (just the origin strings)
	fastify.get(
		"/platform/cors/allowed",
		{
			preHandler: requirePermission(CORS_ORIGIN_PERMISSIONS.READ),
			schema: {
				response: {
					200: AllowedOriginsResponseSchema,
				},
			},
		},
		async (_request, reply) => {
			const origins = await corsAllowedOriginRepository.getAllowedOrigins();
			return jsonSuccess(reply, {
				origins: Array.from(origins),
			});
		},
	);

	// GET /api/admin/platform/cors/:id - Get CORS origin by ID
	fastify.get(
		"/platform/cors/:id",
		{
			preHandler: requirePermission(CORS_ORIGIN_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: CorsOriginResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const origin = await corsAllowedOriginRepository.findById(id);

			if (!origin) {
				return notFound(reply, `CORS origin not found: ${id}`);
			}

			return jsonSuccess(reply, toCorsOriginResponse(origin));
		},
	);

	// DELETE /api/admin/platform/cors/:id - Delete CORS origin
	fastify.delete(
		"/platform/cors/:id",
		{
			preHandler: requirePermission(CORS_ORIGIN_PERMISSIONS.DELETE),
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

			const command: DeleteCorsOriginCommand = {
				originId: id,
			};

			const result = await deleteCorsOriginUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);
}

/**
 * Convert a CorsAllowedOrigin entity to a response.
 */
function toCorsOriginResponse(origin: {
	id: string;
	origin: string;
	description: string | null;
	createdBy: string | null;
	createdAt: Date;
	updatedAt: Date;
}): CorsOriginResponse {
	return {
		id: origin.id,
		origin: origin.origin,
		description: origin.description,
		createdBy: origin.createdBy,
		createdAt: origin.createdAt.toISOString(),
		updatedAt: origin.updatedAt.toISOString(),
	};
}
