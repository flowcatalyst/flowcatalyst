/**
 * Roles BFF API
 *
 * Backend-for-Frontend endpoints for role and permission management.
 * Response shapes match what the Vue frontend expects.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import {
	sendResult,
	jsonCreated,
	jsonSuccess,
	noContent,
	notFound,
	ErrorResponseSchema,
} from '@flowcatalyst/http';
import { Result } from '@flowcatalyst/application';
import type { UseCase } from '@flowcatalyst/application';

import type {
	CreateRoleCommand,
	UpdateRoleCommand,
	DeleteRoleCommand,
} from '../../application/index.js';
import type { RoleCreated, RoleUpdated, RoleDeleted, RoleSource, AuthRole } from '../../domain/index.js';
import { getShortName } from '../../domain/index.js';
import type {
	RoleRepository,
	PermissionRepository,
	ApplicationRepository,
} from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { ROLE_PERMISSIONS, PERMISSION_PERMISSIONS } from '../../authorization/permissions/platform-iam.js';

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
const PermissionParam = Type.Object({ permission: Type.String() });

const RoleListQuerySchema = Type.Object({
	application: Type.Optional(Type.String()),
	source: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const BffRoleSchema = Type.Object({
	id: Type.String(),
	name: Type.String(),
	shortName: Type.String(),
	displayName: Type.String(),
	description: Type.Optional(Type.String()),
	permissions: Type.Array(Type.String()),
	applicationCode: Type.String(),
	source: Type.String(),
	clientManaged: Type.Boolean(),
	createdAt: Type.String({ format: 'date-time' }),
	updatedAt: Type.String({ format: 'date-time' }),
});

const BffRoleListResponseSchema = Type.Object({
	items: Type.Array(BffRoleSchema),
	total: Type.Integer(),
});

const BffApplicationOptionSchema = Type.Object({
	id: Type.String(),
	code: Type.String(),
	name: Type.String(),
});

const BffApplicationOptionsResponseSchema = Type.Object({
	options: Type.Array(BffApplicationOptionSchema),
});

const BffPermissionSchema = Type.Object({
	permission: Type.String(),
	application: Type.String(),
	context: Type.String(),
	aggregate: Type.String(),
	action: Type.String(),
	description: Type.String(),
});

const BffPermissionListResponseSchema = Type.Object({
	items: Type.Array(BffPermissionSchema),
	total: Type.Integer(),
});

type BffRole = Static<typeof BffRoleSchema>;
type BffPermission = Static<typeof BffPermissionSchema>;

/**
 * Dependencies for the roles BFF API.
 */
export interface RolesBffDeps {
	readonly roleRepository: RoleRepository;
	readonly permissionRepository: PermissionRepository;
	readonly applicationRepository: ApplicationRepository;
	readonly createRoleUseCase: UseCase<CreateRoleCommand, RoleCreated>;
	readonly updateRoleUseCase: UseCase<UpdateRoleCommand, RoleUpdated>;
	readonly deleteRoleUseCase: UseCase<DeleteRoleCommand, RoleDeleted>;
}

/**
 * Register role BFF routes.
 */
export async function registerRolesBffRoutes(
	fastify: FastifyInstance,
	deps: RolesBffDeps,
): Promise<void> {
	const {
		roleRepository,
		permissionRepository,
		applicationRepository,
		createRoleUseCase,
		updateRoleUseCase,
		deleteRoleUseCase,
	} = deps;

	// GET /bff/roles - List roles with optional filters
	fastify.get(
		'/roles',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				querystring: RoleListQuerySchema,
				response: {
					200: BffRoleListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof RoleListQuerySchema>;

			let roles: AuthRole[];
			if (query.source) {
				roles = await roleRepository.findBySource(query.source.toUpperCase() as RoleSource);
			} else if (query.application) {
				// Find application by code to get its ID
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
				items: roles.map(toBffRole),
				total: roles.length,
			});
		},
	);

	// GET /bff/roles/filters/applications - Application options for role filter
	fastify.get(
		'/roles/filters/applications',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				response: {
					200: BffApplicationOptionsResponseSchema,
				},
			},
		},
		async (_request, reply) => {
			const applications = await applicationRepository.findAll();
			return jsonSuccess(reply, {
				options: applications.map((app) => ({
					id: app.id,
					code: app.code,
					name: app.name,
				})),
			});
		},
	);

	// GET /bff/roles/permissions - List all permissions
	fastify.get(
		'/roles/permissions',
		{
			preHandler: requirePermission(PERMISSION_PERMISSIONS.READ),
			schema: {
				response: {
					200: BffPermissionListResponseSchema,
				},
			},
		},
		async (_request, reply) => {
			const permissions = await permissionRepository.findAll();

			return jsonSuccess(reply, {
				items: permissions.map(toBffPermission),
				total: permissions.length,
			});
		},
	);

	// GET /bff/roles/permissions/:permission - Get permission by code
	fastify.get(
		'/roles/permissions/:permission',
		{
			preHandler: requirePermission(PERMISSION_PERMISSIONS.READ),
			schema: {
				params: PermissionParam,
				response: {
					200: BffPermissionSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { permission } = request.params as Static<typeof PermissionParam>;
			const allPermissions = await permissionRepository.findAll();
			const found = allPermissions.find((p) => p.code === permission);

			if (!found) {
				return notFound(reply, `Permission not found: ${permission}`);
			}

			return jsonSuccess(reply, toBffPermission(found));
		},
	);

	// GET /bff/roles/:roleName - Get role by name
	fastify.get(
		'/roles/:roleName',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
			schema: {
				params: RoleNameParam,
				response: {
					200: BffRoleSchema,
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

			return jsonSuccess(reply, toBffRole(role));
		},
	);

	// POST /bff/roles - Create role
	fastify.post(
		'/roles',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.CREATE),
			schema: {
				body: CreateRoleSchema,
				response: {
					201: BffRoleSchema,
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
				...(body.permissions !== undefined && { permissions: body.permissions }),
				...(body.clientManaged !== undefined && { clientManaged: body.clientManaged }),
			};

			const result = await createRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const role = await roleRepository.findById(result.value.getData().roleId);
				if (role) {
					return jsonCreated(reply, toBffRole(role));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /bff/roles/:roleName - Update role
	fastify.put(
		'/roles/:roleName',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.UPDATE),
			schema: {
				params: RoleNameParam,
				body: UpdateRoleSchema,
				response: {
					200: BffRoleSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { roleName } = request.params as Static<typeof RoleNameParam>;

			// Find role by name to get its ID
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
				...(body.permissions !== undefined && { permissions: body.permissions }),
				...(body.clientManaged !== undefined && { clientManaged: body.clientManaged }),
			};

			const result = await updateRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const role = await roleRepository.findById(existingRole.id);
				if (role) {
					return jsonSuccess(reply, toBffRole(role));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /bff/roles/:roleName - Delete role
	fastify.delete(
		'/roles/:roleName',
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

			// Find role by name to get its ID
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

function toBffRole(role: AuthRole): BffRole {
	return {
		id: role.id,
		name: role.name,
		shortName: getShortName(role),
		displayName: role.displayName,
		...(role.description ? { description: role.description } : {}),
		permissions: [...role.permissions],
		applicationCode: role.applicationCode ?? '',
		source: role.source,
		clientManaged: role.clientManaged,
		createdAt: role.createdAt.toISOString(),
		updatedAt: role.updatedAt.toISOString(),
	};
}

function toBffPermission(permission: {
	id: string;
	code: string;
	subdomain: string;
	context: string;
	aggregate: string;
	action: string;
	description: string | null;
}): BffPermission {
	return {
		permission: permission.code,
		application: permission.subdomain,
		context: permission.context,
		aggregate: permission.aggregate,
		action: permission.action,
		description: permission.description ?? '',
	};
}
