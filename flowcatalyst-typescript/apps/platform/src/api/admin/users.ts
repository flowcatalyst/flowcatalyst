/**
 * Users Admin API
 *
 * REST endpoints for user management.
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
	CreateUserCommand,
	UpdateUserCommand,
	ActivateUserCommand,
	DeactivateUserCommand,
	DeleteUserCommand,
	AssignRolesCommand,
	GrantClientAccessCommand,
	RevokeClientAccessCommand,
} from '../../application/index.js';
import type {
	UserCreated,
	UserUpdated,
	UserActivated,
	UserDeactivated,
	UserDeleted,
	RolesAssigned,
	ClientAccessGranted,
	ClientAccessRevoked,
} from '../../domain/index.js';
import type {
	PrincipalRepository,
	ClientAccessGrantRepository,
	AnchorDomainRepository,
	ClientAuthConfigRepository,
} from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { USER_PERMISSIONS, CLIENT_ACCESS_PERMISSIONS } from '../../authorization/permissions/platform-iam.js';

// Request schemas using TypeBox
const CreateUserSchema = Type.Object({
	email: Type.String({ format: 'email' }),
	password: Type.Union([Type.String({ minLength: 8 }), Type.Null()]),
	name: Type.String({ minLength: 1 }),
	clientId: Type.Optional(Type.Union([Type.String({ minLength: 13, maxLength: 13 }), Type.Null()])),
});

const UpdateUserSchema = Type.Object({
	name: Type.String({ minLength: 1 }),
});

const AssignRolesSchema = Type.Object({
	roles: Type.Array(Type.String()),
});

const GrantClientAccessSchema = Type.Object({
	clientId: Type.String({ minLength: 17, maxLength: 17 }),
});

type CreateUserBody = Static<typeof CreateUserSchema>;
type UpdateUserBody = Static<typeof UpdateUserSchema>;
type AssignRolesBody = Static<typeof AssignRolesSchema>;
type GrantClientAccessBody = Static<typeof GrantClientAccessSchema>;

// Response schemas for user
interface UserResponse {
	id: string;
	type: string;
	scope: string | null;
	clientId: string | null;
	name: string;
	active: boolean;
	email: string | null;
	emailDomain: string | null;
	idpType: string | null;
	createdAt: string;
	updatedAt: string;
}

interface UsersListResponse {
	users: UserResponse[];
	total: number;
	page: number;
	pageSize: number;
}

interface RoleAssignmentResponse {
	roleName: string;
	assignmentSource: string;
	assignedAt: string;
}

interface UserRolesResponse {
	userId: string;
	roles: RoleAssignmentResponse[];
}

interface ClientAccessGrantResponse {
	id: string;
	clientId: string;
	grantedBy: string;
	grantedAt: string;
}

interface UserClientAccessResponse {
	userId: string;
	grants: ClientAccessGrantResponse[];
}

interface EmailDomainCheckResponse {
	domain: string;
	authProvider: string;
	isAnchorDomain: boolean;
	hasAuthConfig: boolean;
	emailExists: boolean;
	info: string | null;
	warning: string | null;
}

/**
 * Dependencies for the users API.
 */
export interface UsersRoutesDeps {
	readonly principalRepository: PrincipalRepository;
	readonly clientAccessGrantRepository: ClientAccessGrantRepository;
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly clientAuthConfigRepository: ClientAuthConfigRepository;
	readonly createUserUseCase: UseCase<CreateUserCommand, UserCreated>;
	readonly updateUserUseCase: UseCase<UpdateUserCommand, UserUpdated>;
	readonly activateUserUseCase: UseCase<ActivateUserCommand, UserActivated>;
	readonly deactivateUserUseCase: UseCase<DeactivateUserCommand, UserDeactivated>;
	readonly deleteUserUseCase: UseCase<DeleteUserCommand, UserDeleted>;
	readonly assignRolesUseCase: UseCase<AssignRolesCommand, RolesAssigned>;
	readonly grantClientAccessUseCase: UseCase<GrantClientAccessCommand, ClientAccessGranted>;
	readonly revokeClientAccessUseCase: UseCase<RevokeClientAccessCommand, ClientAccessRevoked>;
}

/**
 * Register user admin API routes.
 */
export async function registerUsersRoutes(fastify: FastifyInstance, deps: UsersRoutesDeps): Promise<void> {
	const {
		principalRepository,
		clientAccessGrantRepository,
		anchorDomainRepository,
		clientAuthConfigRepository,
		createUserUseCase,
		updateUserUseCase,
		activateUserUseCase,
		deactivateUserUseCase,
		deleteUserUseCase,
		assignRolesUseCase,
		grantClientAccessUseCase,
		revokeClientAccessUseCase,
	} = deps;

	// POST /api/admin/users - Create user
	fastify.post('/users', async (request, reply) => {
		const bodyResult = safeValidate(request.body, CreateUserSchema);
		if (!bodyResult.success) {
			return badRequest(reply, bodyResult.error);
		}

		const body = bodyResult.data as CreateUserBody;
		const ctx = request.executionContext;

		const command: CreateUserCommand = {
			email: body.email,
			password: body.password,
			name: body.name,
			clientId: body.clientId ?? null,
		};

		const result = await createUserUseCase.execute(command, ctx);

		if (Result.isSuccess(result)) {
			const event = result.value;
			const response: UserResponse = {
				id: event.getData().userId,
				type: 'USER',
				scope: event.getData().scope,
				clientId: event.getData().clientId,
				name: event.getData().name,
				active: true,
				email: event.getData().email,
				emailDomain: event.getData().emailDomain,
				idpType: event.getData().idpType,
				createdAt: event.time.toISOString(),
				updatedAt: event.time.toISOString(),
			};
			return jsonCreated(reply, response);
		}

		return sendResult(reply, result);
	});

	// GET /api/admin/users - List users
	fastify.get('/users', async (request, reply) => {
		const query = request.query as { page?: string; pageSize?: string };
		const page = parseInt(query.page ?? '0', 10);
		const pageSize = Math.min(parseInt(query.pageSize ?? '20', 10), 100);

		const pagedResult = await principalRepository.findPaged(page, pageSize);

		const response: UsersListResponse = {
			users: pagedResult.items
				.filter((p) => p.type === 'USER')
				.map((p) => ({
					id: p.id,
					type: p.type,
					scope: p.scope,
					clientId: p.clientId,
					name: p.name,
					active: p.active,
					email: p.userIdentity?.email ?? null,
					emailDomain: p.userIdentity?.emailDomain ?? null,
					idpType: p.userIdentity?.idpType ?? null,
					createdAt: p.createdAt.toISOString(),
					updatedAt: p.updatedAt.toISOString(),
				})),
			total: pagedResult.totalItems,
			page: pagedResult.page,
			pageSize: pagedResult.pageSize,
		};

		return jsonSuccess(reply, response);
	});

	// GET /api/admin/users/:id - Get user by ID
	fastify.get('/users/:id', async (request, reply) => {
		const { id } = request.params as { id: string };
		const principal = await principalRepository.findById(id);

		if (!principal || principal.type !== 'USER') {
			return notFound(reply, `User not found: ${id}`);
		}

		const response: UserResponse = {
			id: principal.id,
			type: principal.type,
			scope: principal.scope,
			clientId: principal.clientId,
			name: principal.name,
			active: principal.active,
			email: principal.userIdentity?.email ?? null,
			emailDomain: principal.userIdentity?.emailDomain ?? null,
			idpType: principal.userIdentity?.idpType ?? null,
			createdAt: principal.createdAt.toISOString(),
			updatedAt: principal.updatedAt.toISOString(),
		};

		return jsonSuccess(reply, response);
	});

	// PUT /api/admin/users/:id - Update user
	fastify.put('/users/:id', async (request, reply) => {
		const { id } = request.params as { id: string };
		const bodyResult = safeValidate(request.body, UpdateUserSchema);
		if (!bodyResult.success) {
			return badRequest(reply, bodyResult.error);
		}

		const body = bodyResult.data as UpdateUserBody;
		const ctx = request.executionContext;

		const command: UpdateUserCommand = {
			userId: id,
			name: body.name,
		};

		const result = await updateUserUseCase.execute(command, ctx);

		if (Result.isSuccess(result)) {
			const principal = await principalRepository.findById(id);
			if (principal) {
				const response: UserResponse = {
					id: principal.id,
					type: principal.type,
					scope: principal.scope,
					clientId: principal.clientId,
					name: principal.name,
					active: principal.active,
					email: principal.userIdentity?.email ?? null,
					emailDomain: principal.userIdentity?.emailDomain ?? null,
					idpType: principal.userIdentity?.idpType ?? null,
					createdAt: principal.createdAt.toISOString(),
					updatedAt: principal.updatedAt.toISOString(),
				};
				return jsonSuccess(reply, response);
			}
		}

		return sendResult(reply, result);
	});

	// POST /api/admin/users/:id/activate - Activate user
	fastify.post('/users/:id/activate', async (request, reply) => {
		const { id } = request.params as { id: string };
		const ctx = request.executionContext;

		const command: ActivateUserCommand = {
			userId: id,
		};

		const result = await activateUserUseCase.execute(command, ctx);

		if (Result.isSuccess(result)) {
			const principal = await principalRepository.findById(id);
			if (principal) {
				const response: UserResponse = {
					id: principal.id,
					type: principal.type,
					scope: principal.scope,
					clientId: principal.clientId,
					name: principal.name,
					active: principal.active,
					email: principal.userIdentity?.email ?? null,
					emailDomain: principal.userIdentity?.emailDomain ?? null,
					idpType: principal.userIdentity?.idpType ?? null,
					createdAt: principal.createdAt.toISOString(),
					updatedAt: principal.updatedAt.toISOString(),
				};
				return jsonSuccess(reply, response);
			}
		}

		return sendResult(reply, result);
	});

	// POST /api/admin/users/:id/deactivate - Deactivate user
	fastify.post('/users/:id/deactivate', async (request, reply) => {
		const { id } = request.params as { id: string };
		const ctx = request.executionContext;

		const command: DeactivateUserCommand = {
			userId: id,
		};

		const result = await deactivateUserUseCase.execute(command, ctx);

		if (Result.isSuccess(result)) {
			const principal = await principalRepository.findById(id);
			if (principal) {
				const response: UserResponse = {
					id: principal.id,
					type: principal.type,
					scope: principal.scope,
					clientId: principal.clientId,
					name: principal.name,
					active: principal.active,
					email: principal.userIdentity?.email ?? null,
					emailDomain: principal.userIdentity?.emailDomain ?? null,
					idpType: principal.userIdentity?.idpType ?? null,
					createdAt: principal.createdAt.toISOString(),
					updatedAt: principal.updatedAt.toISOString(),
				};
				return jsonSuccess(reply, response);
			}
		}

		return sendResult(reply, result);
	});

	// DELETE /api/admin/users/:id - Delete user
	fastify.delete('/users/:id', async (request, reply) => {
		const { id } = request.params as { id: string };
		const ctx = request.executionContext;

		const command: DeleteUserCommand = {
			userId: id,
		};

		const result = await deleteUserUseCase.execute(command, ctx);

		if (Result.isSuccess(result)) {
			return noContent(reply);
		}

		return sendResult(reply, result);
	});

	// GET /api/admin/users/:id/roles - Get user roles
	fastify.get(
		'/users/:id/roles',
		{
			preHandler: requirePermission(USER_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const principal = await principalRepository.findById(id);

			if (!principal || principal.type !== 'USER') {
				return notFound(reply, `User not found: ${id}`);
			}

			const response: UserRolesResponse = {
				userId: principal.id,
				roles: principal.roles.map((r) => ({
					roleName: r.roleName,
					assignmentSource: r.assignmentSource,
					assignedAt: r.assignedAt.toISOString(),
				})),
			};

			return jsonSuccess(reply, response);
		},
	);

	// PUT /api/admin/users/:id/roles - Assign roles to user
	fastify.put(
		'/users/:id/roles',
		{
			preHandler: requirePermission(USER_PERMISSIONS.ASSIGN_ROLES),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, AssignRolesSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as AssignRolesBody;
			const ctx = request.executionContext;

			const command: AssignRolesCommand = {
				userId: id,
				roles: body.roles,
			};

			const result = await assignRolesUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const principal = await principalRepository.findById(id);
				if (principal) {
					const response: UserRolesResponse = {
						userId: principal.id,
						roles: principal.roles.map((r) => ({
							roleName: r.roleName,
							assignmentSource: r.assignmentSource,
							assignedAt: r.assignedAt.toISOString(),
						})),
					};
					return jsonSuccess(reply, response);
				}
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/users/:id/client-access - Get user client access grants
	fastify.get(
		'/users/:id/client-access',
		{
			preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const principal = await principalRepository.findById(id);

			if (!principal || principal.type !== 'USER') {
				return notFound(reply, `User not found: ${id}`);
			}

			const grants = await clientAccessGrantRepository.findByPrincipal(id);

			const response: UserClientAccessResponse = {
				userId: principal.id,
				grants: grants.map((g) => ({
					id: g.id,
					clientId: g.clientId,
					grantedBy: g.grantedBy,
					grantedAt: g.grantedAt.toISOString(),
				})),
			};

			return jsonSuccess(reply, response);
		},
	);

	// POST /api/admin/users/:id/client-access - Grant client access to user
	fastify.post(
		'/users/:id/client-access',
		{
			preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.GRANT),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, GrantClientAccessSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as GrantClientAccessBody;
			const ctx = request.executionContext;

			const command: GrantClientAccessCommand = {
				userId: id,
				clientId: body.clientId,
			};

			const result = await grantClientAccessUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const grant = await clientAccessGrantRepository.findByPrincipalAndClient(id, body.clientId);
				if (grant) {
					const response: ClientAccessGrantResponse = {
						id: grant.id,
						clientId: grant.clientId,
						grantedBy: grant.grantedBy,
						grantedAt: grant.grantedAt.toISOString(),
					};
					return jsonCreated(reply, response);
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/users/:id/client-access/:clientId - Revoke client access from user
	fastify.delete(
		'/users/:id/client-access/:clientId',
		{
			preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.REVOKE),
		},
		async (request, reply) => {
			const { id, clientId } = request.params as { id: string; clientId: string };
			const ctx = request.executionContext;

			const command: RevokeClientAccessCommand = {
				userId: id,
				clientId,
			};

			const result = await revokeClientAccessUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/users/check-email-domain - Check email domain configuration
	fastify.get(
		'/users/check-email-domain',
		{
			preHandler: requirePermission(USER_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const query = request.query as { email?: string };
			const email = query.email;

			if (!email) {
				return badRequest(reply, 'Email query parameter is required');
			}

			// Validate email format and extract domain
			const atIndex = email.indexOf('@');
			if (atIndex === -1 || atIndex === 0 || atIndex === email.length - 1) {
				return badRequest(reply, 'Invalid email format');
			}
			const domain = email.substring(atIndex + 1).toLowerCase();
			if (!domain || domain.indexOf('.') === -1) {
				return badRequest(reply, 'Invalid email domain');
			}

			// Check if email already exists
			const emailExists = await principalRepository.existsByEmail(email);

			// Check if this is an anchor domain
			const isAnchorDomain = await anchorDomainRepository.existsByDomain(domain);

			// Check for auth configuration
			const authConfig = await clientAuthConfigRepository.findByEmailDomain(domain);
			const hasAuthConfig = authConfig !== undefined;
			const authProvider = authConfig?.authProvider ?? 'INTERNAL';

			// Build info and warning messages
			let info: string | null = null;
			let warning: string | null = null;

			if (isAnchorDomain) {
				info = 'This email domain is configured as an anchor domain. Users will have platform-wide access.';
			} else if (hasAuthConfig && authConfig) {
				if (authConfig.authProvider === 'OIDC') {
					info = `This domain uses external OIDC authentication.`;
				} else {
					info = 'This domain has a configured authentication method.';
				}
			} else {
				info = 'This domain will use internal authentication. Users will be created with a password.';
			}

			if (emailExists) {
				warning = 'A user with this email already exists.';
			}

			const response: EmailDomainCheckResponse = {
				domain,
				authProvider,
				isAnchorDomain,
				hasAuthConfig,
				emailExists,
				info,
				warning,
			};

			return jsonSuccess(reply, response);
		},
	);
}
