/**
 * Roles Admin API
 *
 * REST endpoints for role and permission management.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import {
	sendResult,
	jsonCreated,
	jsonSuccess,
	noContent,
	notFound,
	badRequest,
	safeValidate,
} from '@flowcatalyst/http';
import { Result } from '@flowcatalyst/application';
import type { UseCase } from '@flowcatalyst/application';

import type {
	CreateRoleCommand,
	UpdateRoleCommand,
	DeleteRoleCommand,
} from '../../application/index.js';
import type { RoleCreated, RoleUpdated, RoleDeleted, RoleSource } from '../../domain/index.js';
import type { RoleRepository, PermissionRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { ROLE_PERMISSIONS, PERMISSION_PERMISSIONS } from '../../authorization/permissions/platform-iam.js';

// Request schemas using TypeBox
const CreateRoleSchema = Type.Object({
	applicationId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	applicationCode: Type.Optional(Type.Union([Type.String({ maxLength: 50 }), Type.Null()])),
	shortName: Type.String({ minLength: 1, maxLength: 100 }),
	displayName: Type.String({ minLength: 1, maxLength: 255 }),
	description: Type.Optional(Type.Union([Type.String({ maxLength: 1000 }), Type.Null()])),
	source: Type.Optional(Type.Union([Type.Literal('CODE'), Type.Literal('DATABASE'), Type.Literal('SDK')])),
	permissions: Type.Optional(Type.Array(Type.String())),
	clientManaged: Type.Optional(Type.Boolean()),
});

const UpdateRoleSchema = Type.Object({
	displayName: Type.String({ minLength: 1, maxLength: 255 }),
	description: Type.Optional(Type.Union([Type.String({ maxLength: 1000 }), Type.Null()])),
	permissions: Type.Optional(Type.Array(Type.String())),
	clientManaged: Type.Optional(Type.Boolean()),
});

type CreateRoleBody = Static<typeof CreateRoleSchema>;
type UpdateRoleBody = Static<typeof UpdateRoleSchema>;

// Response schemas
interface RoleResponse {
	id: string;
	applicationId: string | null;
	applicationCode: string | null;
	name: string;
	displayName: string;
	description: string | null;
	source: string;
	permissions: string[];
	clientManaged: boolean;
	createdAt: string;
	updatedAt: string;
}

interface RolesListResponse {
	roles: RoleResponse[];
	total: number;
	page: number;
	pageSize: number;
}

interface PermissionResponse {
	id: string;
	code: string;
	subdomain: string;
	context: string;
	aggregate: string;
	action: string;
	description: string | null;
}

interface PermissionsListResponse {
	permissions: PermissionResponse[];
}

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
export async function registerRolesRoutes(fastify: FastifyInstance, deps: RolesRoutesDeps): Promise<void> {
	const { roleRepository, permissionRepository, createRoleUseCase, updateRoleUseCase, deleteRoleUseCase } = deps;

	// POST /api/admin/roles - Create role
	fastify.post(
		'/roles',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.CREATE),
		},
		async (request, reply) => {
			const bodyResult = safeValidate(request.body, CreateRoleSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as CreateRoleBody;
			const ctx = request.executionContext;

			const command: CreateRoleCommand = {
				applicationId: body.applicationId ?? null,
				applicationCode: body.applicationCode ?? null,
				shortName: body.shortName,
				displayName: body.displayName,
				description: body.description ?? null,
				...(body.source !== undefined && { source: body.source as RoleSource }),
				...(body.permissions !== undefined && { permissions: body.permissions }),
				...(body.clientManaged !== undefined && { clientManaged: body.clientManaged }),
			};

			const result = await createRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const role = await roleRepository.findById(result.value.getData().roleId);
				if (role) {
					return jsonCreated(reply, toRoleResponse(role));
				}
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/roles - List roles
	fastify.get(
		'/roles',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const query = request.query as { page?: string; pageSize?: string; q?: string; source?: string; applicationId?: string };
			const page = parseInt(query.page ?? '0', 10);
			const pageSize = Math.min(parseInt(query.pageSize ?? '20', 10), 100);

			let pagedResult;
			if (query.q) {
				pagedResult = await roleRepository.search(query.q, page, pageSize);
			} else {
				pagedResult = await roleRepository.findPaged(page, pageSize);
			}

			const response: RolesListResponse = {
				roles: pagedResult.items.map(toRoleResponse),
				total: pagedResult.totalItems,
				page: pagedResult.page,
				pageSize: pagedResult.pageSize,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/roles/:id - Get role by ID
	fastify.get(
		'/roles/:id',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const role = await roleRepository.findById(id);

			if (!role) {
				return notFound(reply, `Role not found: ${id}`);
			}

			return jsonSuccess(reply, toRoleResponse(role));
		},
	);

	// GET /api/admin/roles/by-name/:name - Get role by name
	fastify.get(
		'/roles/by-name/:name',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { name } = request.params as { name: string };
			const role = await roleRepository.findByName(name);

			if (!role) {
				return notFound(reply, `Role not found with name: ${name}`);
			}

			return jsonSuccess(reply, toRoleResponse(role));
		},
	);

	// GET /api/admin/roles/by-source/:source - Get roles by source
	fastify.get(
		'/roles/by-source/:source',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { source } = request.params as { source: string };
			const validSources = ['CODE', 'DATABASE', 'SDK'];

			if (!validSources.includes(source.toUpperCase())) {
				return badRequest(reply, `Invalid source. Must be one of: ${validSources.join(', ')}`);
			}

			const roles = await roleRepository.findBySource(source.toUpperCase() as RoleSource);

			return jsonSuccess(reply, {
				roles: roles.map(toRoleResponse),
			});
		},
	);

	// GET /api/admin/roles/by-application/:applicationId - Get roles by application
	fastify.get(
		'/roles/by-application/:applicationId',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { applicationId } = request.params as { applicationId: string };
			const roles = await roleRepository.findByApplicationId(applicationId);

			return jsonSuccess(reply, {
				roles: roles.map(toRoleResponse),
			});
		},
	);

	// PUT /api/admin/roles/:id - Update role
	fastify.put(
		'/roles/:id',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateRoleSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateRoleBody;
			const ctx = request.executionContext;

			const command: UpdateRoleCommand = {
				roleId: id,
				displayName: body.displayName,
				description: body.description ?? null,
				...(body.permissions !== undefined && { permissions: body.permissions }),
				...(body.clientManaged !== undefined && { clientManaged: body.clientManaged }),
			};

			const result = await updateRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const role = await roleRepository.findById(id);
				if (role) {
					return jsonSuccess(reply, toRoleResponse(role));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/roles/:id - Delete role
	fastify.delete(
		'/roles/:id',
		{
			preHandler: requirePermission(ROLE_PERMISSIONS.DELETE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const ctx = request.executionContext;

			const command: DeleteRoleCommand = {
				roleId: id,
			};

			const result = await deleteRoleUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/permissions - List all permissions
	fastify.get(
		'/permissions',
		{
			preHandler: requirePermission(PERMISSION_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const permissions = await permissionRepository.findAll();

			const response: PermissionsListResponse = {
				permissions: permissions.map(toPermissionResponse),
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/permissions/by-subdomain/:subdomain - List permissions by subdomain
	fastify.get(
		'/permissions/by-subdomain/:subdomain',
		{
			preHandler: requirePermission(PERMISSION_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { subdomain } = request.params as { subdomain: string };
			const permissions = await permissionRepository.findBySubdomain(subdomain);

			const response: PermissionsListResponse = {
				permissions: permissions.map(toPermissionResponse),
			};

			return jsonSuccess(reply, response);
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
