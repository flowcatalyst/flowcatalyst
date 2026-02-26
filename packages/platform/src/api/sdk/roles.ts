/**
 * SDK Roles API
 *
 * REST endpoints for role management via external SDKs.
 * Uses Bearer token authentication (not BFF session).
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
	CreateRoleCommand,
	UpdateRoleCommand,
	DeleteRoleCommand,
} from "../../application/index.js";
import type {
	RoleCreated,
	RoleUpdated,
	RoleDeleted,
	RoleSource,
	AuthRole,
} from "../../domain/index.js";
import { getShortName } from "../../domain/index.js";
import type {
	RoleRepository,
	ApplicationRepository,
} from "../../infrastructure/persistence/index.js";
import { requirePermission } from "../../authorization/index.js";
import { ROLE_PERMISSIONS } from "../../authorization/permissions/platform-iam.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateRoleSchema = Type.Object({
	applicationCode: Type.String({ minLength: 1, maxLength: 50 }),
	name: Type.String({ minLength: 1, maxLength: 100 }),
	displayName: Type.Optional(Type.String({ minLength: 1, maxLength: 255 })),
	description: Type.Optional(Type.String({ maxLength: 1000 })),
	permissions: Type.Optional(Type.Array(Type.String())),
	clientManaged: Type.Optional(Type.Boolean()),
});

const UpdateRoleSchema = Type.Object({
	displayName: Type.Optional(Type.String({ minLength: 1, maxLength: 255 })),
	description: Type.Optional(Type.String({ maxLength: 1000 })),
	permissions: Type.Optional(Type.Array(Type.String())),
	clientManaged: Type.Optional(Type.Boolean()),
});

const RoleNameParam = Type.Object({ roleName: Type.String() });

const RoleListQuerySchema = Type.Object({
	application: Type.Optional(Type.String()),
	source: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const SdkRoleResponseSchema = Type.Object({
	name: Type.String(),
	applicationCode: Type.String(),
	displayName: Type.String(),
	shortName: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	permissions: Type.Array(Type.String()),
	source: Type.String(),
	clientManaged: Type.Boolean(),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const SdkRoleListResponseSchema = Type.Object({
	roles: Type.Array(SdkRoleResponseSchema),
	total: Type.Integer(),
});

type SdkRoleResponse = Static<typeof SdkRoleResponseSchema>;

/**
 * Dependencies for the SDK roles API.
 */
export interface SdkRolesDeps {
	readonly roleRepository: RoleRepository;
	readonly applicationRepository: ApplicationRepository;
	readonly createRoleUseCase: UseCase<CreateRoleCommand, RoleCreated>;
	readonly updateRoleUseCase: UseCase<UpdateRoleCommand, RoleUpdated>;
	readonly deleteRoleUseCase: UseCase<DeleteRoleCommand, RoleDeleted>;
}

/**
 * Register SDK role routes.
 */
export async function registerSdkRolesRoutes(
	fastify: FastifyInstance,
	deps: SdkRolesDeps,
): Promise<void> {
	const {
		roleRepository,
		applicationRepository,
		createRoleUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
	} = deps;

	// GET /api/sdk/roles - List roles with optional filters
	fastify.get(
		"/roles",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				querystring: RoleListQuerySchema,
				response: {
					200: SdkRoleListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof RoleListQuerySchema>;

			let roles: AuthRole[];
			if (query.source) {
				roles = await roleRepository.findBySource(
					query.source.toUpperCase() as RoleSource,
				);
			} else if (query.application) {
				const app = await applicationRepository.findByCode(query.application);
				if (app) {
					roles = await roleRepository.findByApplicationId(app.id);
				} else {
					roles = [];
				}
			} else {
				roles = await roleRepository.findAll();
			}

			return jsonSuccess(reply, {
				roles: roles.map(toSdkRole),
				total: roles.length,
			});
		},
	);

	// GET /api/sdk/roles/:roleName - Get role by name
	fastify.get(
		"/roles/:roleName",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				params: RoleNameParam,
				response: {
					200: SdkRoleResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { roleName } = request.params as Static<typeof RoleNameParam>;
			const role = await roleRepository.findByName(roleName);

			if (!role) {
				return notFound(reply, `Role not found: ${roleName}`);
			}

			return jsonSuccess(reply, toSdkRole(role));
		},
	);

	// POST /api/sdk/roles - Create role
	fastify.post(
		"/roles",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.CREATE),
			schema: {
				body: CreateRoleSchema,
				response: {
					201: SdkRoleResponseSchema,
					400: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as Static<typeof CreateRoleSchema>;
			const ctx = request.executionContext;

			const command: CreateRoleCommand = {
				applicationCode: body.applicationCode,
				shortName: body.name,
				displayName: body.displayName ?? body.name,
				description: body.description ?? null,
				source: "SDK" as RoleSource,
				...(body.permissions !== undefined && {
					permissions: body.permissions,
				}),
				...(body.clientManaged !== undefined && {
					clientManaged: body.clientManaged,
				}),
			};

			const result = await createRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const role = await roleRepository.findById(
					result.value.getData().roleId,
				);
				if (role) {
					return jsonCreated(reply, toSdkRole(role));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /api/sdk/roles/:roleName - Update role
	fastify.put(
		"/roles/:roleName",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.UPDATE),
			schema: {
				params: RoleNameParam,
				body: UpdateRoleSchema,
				response: {
					200: SdkRoleResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { roleName } = request.params as Static<typeof RoleNameParam>;

			const existingRole = await roleRepository.findByName(roleName);
			if (!existingRole) {
				return notFound(reply, `Role not found: ${roleName}`);
			}

			const body = request.body as Static<typeof UpdateRoleSchema>;
			const ctx = request.executionContext;

			const command: UpdateRoleCommand = {
				roleId: existingRole.id,
				displayName: body.displayName ?? existingRole.displayName,
				description: body.description ?? null,
				...(body.permissions !== undefined && {
					permissions: body.permissions,
				}),
				...(body.clientManaged !== undefined && {
					clientManaged: body.clientManaged,
				}),
			};

			const result = await updateRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const role = await roleRepository.findById(existingRole.id);
				if (role) {
					return jsonSuccess(reply, toSdkRole(role));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/sdk/roles/:roleName - Delete role
	fastify.delete(
		"/roles/:roleName",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.DELETE),
			schema: {
				params: RoleNameParam,
				response: {
					204: Type.Null(),
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { roleName } = request.params as Static<typeof RoleNameParam>;

			const existingRole = await roleRepository.findByName(roleName);
			if (!existingRole) {
				return notFound(reply, `Role not found: ${roleName}`);
			}

			const ctx = request.executionContext;
			const command: DeleteRoleCommand = { roleId: existingRole.id };
			const result = await deleteRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toSdkRole(role: AuthRole): SdkRoleResponse {
	return {
		name: role.name,
		applicationCode: role.applicationCode ?? "",
		displayName: role.displayName,
		shortName: getShortName(role),
		description: role.description,
		permissions: [...role.permissions],
		source: role.source,
		clientManaged: role.clientManaged,
		createdAt: role.createdAt.toISOString(),
		updatedAt: role.updatedAt.toISOString(),
	};
}
