/**
 * Roles Admin API
 *
 * REST endpoints for role and permission management.
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import {
	sendResult,
	jsonCreated,
	jsonSuccess,
	noContent,
	notFound,
	badRequest,
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
} from "../../domain/index.js";
import type {
	RoleRepository,
	PermissionRepository,
} from "../../infrastructure/persistence/index.js";
import { requirePermission } from "../../authorization/index.js";
import {
	ROLE_PERMISSIONS,
	PERMISSION_PERMISSIONS,
} from "../../authorization/permissions/platform-iam.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateRoleSchema = Type.Object({
	applicationId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	applicationCode: Type.Optional(
		Type.Union([Type.String({ maxLength: 50 }), Type.Null()]),
	),
	shortName: Type.String({ minLength: 1, maxLength: 100 }),
	displayName: Type.String({ minLength: 1, maxLength: 255 }),
	description: Type.Optional(
		Type.Union([Type.String({ maxLength: 1000 }), Type.Null()]),
	),
	source: Type.Optional(
		Type.Union([
			Type.Literal("CODE"),
			Type.Literal("DATABASE"),
			Type.Literal("SDK"),
		]),
	),
	permissions: Type.Optional(Type.Array(Type.String())),
	clientManaged: Type.Optional(Type.Boolean()),
});

const UpdateRoleSchema = Type.Object({
	displayName: Type.String({ minLength: 1, maxLength: 255 }),
	description: Type.Optional(
		Type.Union([Type.String({ maxLength: 1000 }), Type.Null()]),
	),
	permissions: Type.Optional(Type.Array(Type.String())),
	clientManaged: Type.Optional(Type.Boolean()),
});

const NameParam = Type.Object({ name: Type.String() });
const SourceParam = Type.Object({ source: Type.String() });
const ApplicationIdParam = Type.Object({ applicationId: Type.String() });
const PermissionParam = Type.Object({ permission: Type.String() });

const ListRolesQuery = Type.Object({
	page: Type.Optional(Type.String()),
	pageSize: Type.Optional(Type.String()),
	q: Type.Optional(Type.String()),
	source: Type.Optional(Type.String()),
	applicationId: Type.Optional(Type.String()),
});

type CreateRoleBody = Static<typeof CreateRoleSchema>;
type UpdateRoleBody = Static<typeof UpdateRoleSchema>;

// ─── Response Schemas ───────────────────────────────────────────────────────

const RoleResponseSchema = Type.Object({
	id: Type.String(),
	applicationId: Type.Union([Type.String(), Type.Null()]),
	applicationCode: Type.Union([Type.String(), Type.Null()]),
	name: Type.String(),
	displayName: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	source: Type.String(),
	permissions: Type.Array(Type.String()),
	clientManaged: Type.Boolean(),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const RolesListResponseSchema = Type.Object({
	roles: Type.Array(RoleResponseSchema),
	total: Type.Integer(),
	page: Type.Integer(),
	pageSize: Type.Integer(),
});

const RolesArrayResponseSchema = Type.Object({
	roles: Type.Array(RoleResponseSchema),
});

const PermissionResponseSchema = Type.Object({
	id: Type.String(),
	code: Type.String(),
	subdomain: Type.String(),
	context: Type.String(),
	aggregate: Type.String(),
	action: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
});

const PermissionsListResponseSchema = Type.Object({
	permissions: Type.Array(PermissionResponseSchema),
});

type RoleResponse = Static<typeof RoleResponseSchema>;
type PermissionResponse = Static<typeof PermissionResponseSchema>;

/**
 * Dependencies for the roles API.
 */
export interface RolesRoutesDeps {
	readonly roleRepository: RoleRepository;
	readonly permissionRepository: PermissionRepository;
	readonly createRoleUseCase: UseCase<CreateRoleCommand, RoleCreated>;
	readonly updateRoleUseCase: UseCase<UpdateRoleCommand, RoleUpdated>;
	readonly deleteRoleUseCase: UseCase<DeleteRoleCommand, RoleDeleted>;
}

/**
 * Register role admin API routes.
 */
export async function registerRolesRoutes(
	fastify: FastifyInstance,
	deps: RolesRoutesDeps,
): Promise<void> {
	const {
		roleRepository,
		permissionRepository,
		createRoleUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
	} = deps;

	// POST /api/admin/roles - Create role
	fastify.post(
		"/roles",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.CREATE),
			schema: {
				body: CreateRoleSchema,
				response: {
					201: RoleResponseSchema,
					400: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as CreateRoleBody;
			const ctx = request.executionContext;

			const command: CreateRoleCommand = {
				applicationId: body.applicationId ?? null,
				applicationCode: body.applicationCode ?? null,
				shortName: body.shortName,
				displayName: body.displayName,
				description: body.description ?? null,
				...(body.source !== undefined && { source: body.source as RoleSource }),
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
					return jsonCreated(reply, toRoleResponse(role));
				}
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/roles - List roles
	fastify.get(
		"/roles",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				querystring: ListRolesQuery,
				response: {
					200: RolesListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof ListRolesQuery>;
			const page = parseInt(query.page ?? "0", 10);
			const pageSize = Math.min(parseInt(query.pageSize ?? "20", 10), 100);

			let pagedResult;
			if (query.q) {
				pagedResult = await roleRepository.search(query.q, page, pageSize);
			} else {
				pagedResult = await roleRepository.findPaged(page, pageSize);
			}

			return jsonSuccess(reply, {
				roles: pagedResult.items.map(toRoleResponse),
				total: pagedResult.totalItems,
				page: pagedResult.page,
				pageSize: pagedResult.pageSize,
			});
		},
	);

	// GET /api/admin/roles/:name - Get role by name (Java parity: /roles/{roleName})
	fastify.get(
		"/roles/:name",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				params: NameParam,
				response: {
					200: RoleResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { name } = request.params as Static<typeof NameParam>;
			const role = await roleRepository.findByName(name);

			if (!role) {
				return notFound(reply, `Role not found: ${name}`);
			}

			return jsonSuccess(reply, toRoleResponse(role));
		},
	);

	// GET /api/admin/roles/by-source/:source - Get roles by source
	fastify.get(
		"/roles/by-source/:source",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				params: SourceParam,
				response: {
					200: RolesArrayResponseSchema,
					400: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { source } = request.params as Static<typeof SourceParam>;
			const validSources = ["CODE", "DATABASE", "SDK"];

			if (!validSources.includes(source.toUpperCase())) {
				return badRequest(
					reply,
					`Invalid source. Must be one of: ${validSources.join(", ")}`,
				);
			}

			const roles = await roleRepository.findBySource(
				source.toUpperCase() as RoleSource,
			);

			return jsonSuccess(reply, {
				roles: roles.map(toRoleResponse),
			});
		},
	);

	// GET /api/admin/roles/by-application/:applicationId - Get roles by application
	fastify.get(
		"/roles/by-application/:applicationId",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				params: ApplicationIdParam,
				response: {
					200: RolesArrayResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { applicationId } = request.params as Static<
				typeof ApplicationIdParam
			>;
			const roles = await roleRepository.findByApplicationId(applicationId);

			return jsonSuccess(reply, {
				roles: roles.map(toRoleResponse),
			});
		},
	);

	// PUT /api/admin/roles/:name - Update role by name (Java parity: /roles/{roleName})
	fastify.put(
		"/roles/:name",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.UPDATE),
			schema: {
				params: NameParam,
				body: UpdateRoleSchema,
				response: {
					200: RoleResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { name } = request.params as Static<typeof NameParam>;
			const body = request.body as UpdateRoleBody;
			const ctx = request.executionContext;

			const role = await roleRepository.findByName(name);
			if (!role) {
				return notFound(reply, `Role not found: ${name}`);
			}

			const command: UpdateRoleCommand = {
				roleId: role.id,
				displayName: body.displayName,
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
				const updated = await roleRepository.findById(role.id);
				if (updated) {
					return jsonSuccess(reply, toRoleResponse(updated));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/roles/:name - Delete role by name (Java parity: /roles/{roleName})
	fastify.delete(
		"/roles/:name",
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.DELETE),
			schema: {
				params: NameParam,
				response: {
					204: Type.Null(),
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { name } = request.params as Static<typeof NameParam>;
			const ctx = request.executionContext;

			const role = await roleRepository.findByName(name);
			if (!role) {
				return notFound(reply, `Role not found: ${name}`);
			}

			const command: DeleteRoleCommand = {
				roleId: role.id,
			};

			const result = await deleteRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/roles/permissions - List all permissions (Java parity: nested under /roles)
	fastify.get(
		"/roles/permissions",
		{
			preHandler: requirePermission(PERMISSION_PERMISSIONS.READ),
			schema: {
				response: {
					200: PermissionsListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const permissions = await permissionRepository.findAll();

			return jsonSuccess(reply, {
				permissions: permissions.map(toPermissionResponse),
			});
		},
	);

	// GET /api/admin/roles/permissions/:permission - Get permission by code (Java parity)
	fastify.get(
		"/roles/permissions/:permission",
		{
			preHandler: requirePermission(PERMISSION_PERMISSIONS.READ),
			schema: {
				params: PermissionParam,
				response: {
					200: PermissionResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { permission: code } = request.params as Static<
				typeof PermissionParam
			>;
			const perm = await permissionRepository.findByCode(code);

			if (!perm) {
				return notFound(reply, `Permission not found: ${code}`);
			}

			return jsonSuccess(reply, toPermissionResponse(perm));
		},
	);
}

/**
 * Convert an AuthRole entity to a RoleResponse.
 */
function toRoleResponse(role: {
	id: string;
	applicationId: string | null;
	applicationCode: string | null;
	name: string;
	displayName: string;
	description: string | null;
	source: string;
	permissions: readonly string[];
	clientManaged: boolean;
	createdAt: Date;
	updatedAt: Date;
}): RoleResponse {
	return {
		id: role.id,
		applicationId: role.applicationId,
		applicationCode: role.applicationCode,
		name: role.name,
		displayName: role.displayName,
		description: role.description,
		source: role.source,
		permissions: [...role.permissions],
		clientManaged: role.clientManaged,
		createdAt: role.createdAt.toISOString(),
		updatedAt: role.updatedAt.toISOString(),
	};
}

/**
 * Convert an AuthPermission entity to a PermissionResponse.
 */
function toPermissionResponse(permission: {
	id: string;
	code: string;
	subdomain: string;
	context: string;
	aggregate: string;
	action: string;
	description: string | null;
}): PermissionResponse {
	return {
		id: permission.id,
		code: permission.code,
		subdomain: permission.subdomain,
		context: permission.context,
		aggregate: permission.aggregate,
		action: permission.action,
		description: permission.description,
	};
}
