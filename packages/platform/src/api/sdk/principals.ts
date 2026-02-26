/**
 * SDK Principals API
 *
 * REST endpoints for principal (user/service) management via external SDKs.
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
	CreateUserCommand,
	UpdateUserCommand,
	ActivateUserCommand,
	DeactivateUserCommand,
	AssignRolesCommand,
	GrantClientAccessCommand,
	RevokeClientAccessCommand,
} from "../../application/index.js";
import type {
	UserCreated,
	UserUpdated,
	UserActivated,
	UserDeactivated,
	RolesAssigned,
	ClientAccessGranted,
	ClientAccessRevoked,
	Principal,
	PrincipalType,
} from "../../domain/index.js";
import type {
	PrincipalRepository,
	ClientAccessGrantRepository,
} from "../../infrastructure/persistence/index.js";
import { requirePermission } from "../../authorization/index.js";
import {
	USER_PERMISSIONS,
	CLIENT_ACCESS_PERMISSIONS,
} from "../../authorization/permissions/platform-iam.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateUserSchema = Type.Object({
	email: Type.String({ format: "email" }),
	password: Type.Optional(
		Type.Union([Type.String({ minLength: 8 }), Type.Null()]),
	),
	name: Type.String({ minLength: 1 }),
	clientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const UpdatePrincipalSchema = Type.Object({
	name: Type.String({ minLength: 1 }),
	scope: Type.Optional(Type.String()),
	clientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const AssignRolesSchema = Type.Object({
	roles: Type.Array(Type.String()),
});

const IdParam = Type.Object({ id: Type.String() });
const IdClientParam = Type.Object({
	id: Type.String(),
	clientId: Type.String(),
});

const PrincipalListQuerySchema = Type.Object({
	clientId: Type.Optional(Type.String()),
	type: Type.Optional(Type.String()),
	active: Type.Optional(Type.String()),
	email: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const SdkPrincipalResponseSchema = Type.Object({
	id: Type.String(),
	type: Type.String(),
	scope: Type.Union([Type.String(), Type.Null()]),
	clientId: Type.Union([Type.String(), Type.Null()]),
	name: Type.String(),
	active: Type.Boolean(),
	email: Type.Union([Type.String(), Type.Null()]),
	idpType: Type.Union([Type.String(), Type.Null()]),
	roles: Type.Array(Type.String()),
	grantedClientIds: Type.Array(Type.String()),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const SdkPrincipalListResponseSchema = Type.Object({
	principals: Type.Array(SdkPrincipalResponseSchema),
	total: Type.Integer(),
});

const SdkRoleAssignmentResponseSchema = Type.Object({
	roleName: Type.String(),
	assignmentSource: Type.String(),
	assignedAt: Type.String({ format: "date-time" }),
});

const SdkRolesAssignedResponseSchema = Type.Object({
	roles: Type.Array(SdkRoleAssignmentResponseSchema),
});

const SdkClientAccessGrantResponseSchema = Type.Object({
	id: Type.String(),
	clientId: Type.String(),
	grantedAt: Type.String({ format: "date-time" }),
});

const SdkClientAccessListResponseSchema = Type.Object({
	grants: Type.Array(SdkClientAccessGrantResponseSchema),
});

const MessageResponseSchema = Type.Object({
	message: Type.String(),
});

type SdkPrincipalResponse = Static<typeof SdkPrincipalResponseSchema>;

/**
 * Dependencies for the SDK principals API.
 */
export interface SdkPrincipalsDeps {
	readonly principalRepository: PrincipalRepository;
	readonly clientAccessGrantRepository: ClientAccessGrantRepository;
	readonly createUserUseCase: UseCase<CreateUserCommand, UserCreated>;
	readonly updateUserUseCase: UseCase<UpdateUserCommand, UserUpdated>;
	readonly activateUserUseCase: UseCase<ActivateUserCommand, UserActivated>;
	readonly deactivateUserUseCase: UseCase<
		DeactivateUserCommand,
		UserDeactivated
	>;
	readonly assignRolesUseCase: UseCase<AssignRolesCommand, RolesAssigned>;
	readonly grantClientAccessUseCase: UseCase<
		GrantClientAccessCommand,
		ClientAccessGranted
	>;
	readonly revokeClientAccessUseCase: UseCase<
		RevokeClientAccessCommand,
		ClientAccessRevoked
	>;
}

/**
 * Register SDK principal routes.
 */
export async function registerSdkPrincipalsRoutes(
	fastify: FastifyInstance,
	deps: SdkPrincipalsDeps,
): Promise<void> {
	const {
		principalRepository,
		clientAccessGrantRepository,
		createUserUseCase,
		updateUserUseCase,
		activateUserUseCase,
		deactivateUserUseCase,
		assignRolesUseCase,
		grantClientAccessUseCase,
		revokeClientAccessUseCase,
	} = deps;

	// GET /api/sdk/principals - List principals with optional filters
	fastify.get(
		"/principals",
		{
			preHandler: requirePermission(USER_PERMISSIONS.READ),
			schema: {
				querystring: PrincipalListQuerySchema,
				response: {
					200: SdkPrincipalListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof PrincipalListQuerySchema>;

			let principals: Principal[];

			if (query.email) {
				const principal = await principalRepository.findByEmail(query.email);
				principals = principal ? [principal] : [];
			} else if (query.clientId) {
				principals = await principalRepository.findByClientId(query.clientId);
			} else if (query.type) {
				principals = await principalRepository.findByType(
					query.type.toUpperCase() as PrincipalType,
				);
			} else {
				const paged = await principalRepository.findPaged(0, 1000);
				principals = paged.items;
			}

			// Apply additional filters
			if (query.active !== undefined) {
				const isActive = query.active === "true";
				principals = principals.filter((p) => p.active === isActive);
			}
			if (query.type && !query.email && !query.clientId) {
				// Already filtered by type in the query
			} else if (query.type) {
				const type = query.type.toUpperCase();
				principals = principals.filter((p) => p.type === type);
			}

			// Fetch client access grants for each principal
			const items = await Promise.all(
				principals.map(async (p) => {
					const grants = await clientAccessGrantRepository.findByPrincipal(
						p.id,
					);
					return toSdkPrincipal(
						p,
						grants.map((g) => g.clientId),
					);
				}),
			);

			return jsonSuccess(reply, {
				principals: items,
				total: items.length,
			});
		},
	);

	// GET /api/sdk/principals/:id - Get principal by ID
	fastify.get(
		"/principals/:id",
		{
			preHandler: requirePermission(USER_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: SdkPrincipalResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const principal = await principalRepository.findById(id);

			if (!principal) {
				return notFound(reply, `Principal not found: ${id}`);
			}

			const grants = await clientAccessGrantRepository.findByPrincipal(id);
			return jsonSuccess(
				reply,
				toSdkPrincipal(
					principal,
					grants.map((g) => g.clientId),
				),
			);
		},
	);

	// POST /api/sdk/principals/user - Create user principal
	fastify.post(
		"/principals/user",
		{
			preHandler: requirePermission(USER_PERMISSIONS.CREATE),
			schema: {
				body: CreateUserSchema,
				response: {
					201: SdkPrincipalResponseSchema,
					400: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as Static<typeof CreateUserSchema>;
			const ctx = request.executionContext;

			const command: CreateUserCommand = {
				email: body.email,
				password: body.password ?? null,
				name: body.name,
				clientId: body.clientId ?? null,
			};

			const result = await createUserUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const event = result.value;
				const data = event.getData();
				const response: SdkPrincipalResponse = {
					id: data.userId,
					type: "USER",
					scope: data.scope,
					clientId: data.clientId,
					name: data.name,
					active: true,
					email: data.email,
					idpType: data.idpType,
					roles: [],
					grantedClientIds: [],
					createdAt: event.time.toISOString(),
					updatedAt: event.time.toISOString(),
				};
				return jsonCreated(reply, response);
			}

			return sendResult(reply, result);
		},
	);

	// PUT /api/sdk/principals/:id - Update principal
	fastify.put(
		"/principals/:id",
		{
			preHandler: requirePermission(USER_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				body: UpdatePrincipalSchema,
				response: {
					200: SdkPrincipalResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const body = request.body as Static<typeof UpdatePrincipalSchema>;
			const ctx = request.executionContext;

			const command: UpdateUserCommand = {
				userId: id,
				name: body.name,
				scope: body.scope,
				clientId: body.clientId,
			};

			const result = await updateUserUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const principal = await principalRepository.findById(id);
				if (principal) {
					const grants = await clientAccessGrantRepository.findByPrincipal(id);
					return jsonSuccess(
						reply,
						toSdkPrincipal(
							principal,
							grants.map((g) => g.clientId),
						),
					);
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/sdk/principals/:id/activate - Activate principal
	fastify.post(
		"/principals/:id/activate",
		{
			preHandler: requirePermission(USER_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				response: {
					200: MessageResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const ctx = request.executionContext;

			const command: ActivateUserCommand = { userId: id };
			const result = await activateUserUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return jsonSuccess(reply, { message: "Principal activated" });
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/sdk/principals/:id/deactivate - Deactivate principal
	fastify.post(
		"/principals/:id/deactivate",
		{
			preHandler: requirePermission(USER_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				response: {
					200: MessageResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const ctx = request.executionContext;

			const command: DeactivateUserCommand = { userId: id };
			const result = await deactivateUserUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return jsonSuccess(reply, { message: "Principal deactivated" });
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/sdk/principals/:id/roles - Get principal's roles
	fastify.get(
		"/principals/:id/roles",
		{
			preHandler: requirePermission(USER_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: SdkRolesAssignedResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const principal = await principalRepository.findById(id);

			if (!principal) {
				return notFound(reply, `Principal not found: ${id}`);
			}

			return jsonSuccess(reply, {
				roles: principal.roles.map((r) => ({
					roleName: r.roleName,
					assignmentSource: r.assignmentSource,
					assignedAt: r.assignedAt.toISOString(),
				})),
			});
		},
	);

	// PUT /api/sdk/principals/:id/roles - Declarative batch role assignment
	fastify.put(
		"/principals/:id/roles",
		{
			preHandler: requirePermission(USER_PERMISSIONS.ASSIGN_ROLES),
			schema: {
				params: IdParam,
				body: AssignRolesSchema,
				response: {
					200: SdkRolesAssignedResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const body = request.body as Static<typeof AssignRolesSchema>;
			const ctx = request.executionContext;

			const command: AssignRolesCommand = {
				userId: id,
				roles: body.roles,
			};

			const result = await assignRolesUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const principal = await principalRepository.findById(id);
				if (principal) {
					return jsonSuccess(reply, {
						roles: principal.roles.map((r) => ({
							roleName: r.roleName,
							assignmentSource: r.assignmentSource,
							assignedAt: r.assignedAt.toISOString(),
						})),
					});
				}
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/sdk/principals/:id/clients - Get client access grants
	fastify.get(
		"/principals/:id/clients",
		{
			preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: SdkClientAccessListResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const principal = await principalRepository.findById(id);

			if (!principal) {
				return notFound(reply, `Principal not found: ${id}`);
			}

			const grants = await clientAccessGrantRepository.findByPrincipal(id);

			return jsonSuccess(reply, {
				grants: grants.map((g) => ({
					id: g.id,
					clientId: g.clientId,
					grantedAt: g.grantedAt.toISOString(),
				})),
			});
		},
	);

	// POST /api/sdk/principals/:id/clients/:clientId - Grant client access
	fastify.post(
		"/principals/:id/clients/:clientId",
		{
			preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.GRANT),
			schema: {
				params: IdClientParam,
				response: {
					201: SdkClientAccessGrantResponseSchema,
					404: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id, clientId } = request.params as Static<typeof IdClientParam>;
			const ctx = request.executionContext;

			const command: GrantClientAccessCommand = {
				userId: id,
				clientId,
			};

			const result = await grantClientAccessUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const grant =
					await clientAccessGrantRepository.findByPrincipalAndClient(
						id,
						clientId,
					);
				if (grant) {
					return jsonCreated(reply, {
						id: grant.id,
						clientId: grant.clientId,
						grantedAt: grant.grantedAt.toISOString(),
					});
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/sdk/principals/:id/clients/:clientId - Revoke client access
	fastify.delete(
		"/principals/:id/clients/:clientId",
		{
			preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.REVOKE),
			schema: {
				params: IdClientParam,
				response: {
					204: Type.Null(),
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id, clientId } = request.params as Static<typeof IdClientParam>;
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
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toSdkPrincipal(
	principal: Principal,
	grantedClientIds: string[],
): SdkPrincipalResponse {
	return {
		id: principal.id,
		type: principal.type,
		scope: principal.scope,
		clientId: principal.clientId,
		name: principal.name,
		active: principal.active,
		email: principal.userIdentity?.email ?? null,
		idpType: principal.userIdentity?.idpType ?? null,
		roles: principal.roles.map((r) => r.roleName),
		grantedClientIds,
		createdAt: principal.createdAt.toISOString(),
		updatedAt: principal.updatedAt.toISOString(),
	};
}
