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
  ErrorResponseSchema,
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
  AssignApplicationAccessCommand,
} from '../../application/index.js';
import type {
  UserCreated,
  UserUpdated,
  UserActivated,
  UserDeactivated,
  UserDeleted,
  RolesAssigned,
  ApplicationAccessAssigned,
  ClientAccessGranted,
  ClientAccessRevoked,
} from '../../domain/index.js';
import type {
  PrincipalRepository,
  ClientAccessGrantRepository,
  AnchorDomainRepository,
  ClientAuthConfigRepository,
  ApplicationClientConfigRepository,
} from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import {
  USER_PERMISSIONS,
  CLIENT_ACCESS_PERMISSIONS,
} from '../../authorization/permissions/platform-iam.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

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

// ─── Param Schemas ──────────────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });
const IdClientParam = Type.Object({ id: Type.String(), clientId: Type.String() });

// ─── Query Schemas ──────────────────────────────────────────────────────────

const UsersListQuery = Type.Object({
  page: Type.Optional(Type.String()),
  pageSize: Type.Optional(Type.String()),
});

const EmailDomainCheckQuery = Type.Object({
  email: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const UserResponseSchema = Type.Object({
  id: Type.String(),
  type: Type.String(),
  scope: Type.Union([Type.String(), Type.Null()]),
  clientId: Type.Union([Type.String(), Type.Null()]),
  name: Type.String(),
  active: Type.Boolean(),
  email: Type.Union([Type.String(), Type.Null()]),
  emailDomain: Type.Union([Type.String(), Type.Null()]),
  idpType: Type.Union([Type.String(), Type.Null()]),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const UsersListResponseSchema = Type.Object({
  users: Type.Array(UserResponseSchema),
  total: Type.Integer(),
  page: Type.Integer(),
  pageSize: Type.Integer(),
});

const RoleAssignmentResponseSchema = Type.Object({
  roleName: Type.String(),
  assignmentSource: Type.String(),
  assignedAt: Type.String({ format: 'date-time' }),
});

const UserRolesResponseSchema = Type.Object({
  userId: Type.String(),
  roles: Type.Array(RoleAssignmentResponseSchema),
});

const ClientAccessGrantResponseSchema = Type.Object({
  id: Type.String(),
  clientId: Type.String(),
  grantedBy: Type.String(),
  grantedAt: Type.String({ format: 'date-time' }),
});

const UserClientAccessResponseSchema = Type.Object({
  userId: Type.String(),
  grants: Type.Array(ClientAccessGrantResponseSchema),
});

const EmailDomainCheckResponseSchema = Type.Object({
  domain: Type.String(),
  authProvider: Type.String(),
  isAnchorDomain: Type.Boolean(),
  hasAuthConfig: Type.Boolean(),
  emailExists: Type.Boolean(),
  info: Type.Union([Type.String(), Type.Null()]),
  warning: Type.Union([Type.String(), Type.Null()]),
});

type UserResponse = Static<typeof UserResponseSchema>;

/**
 * Dependencies for the users API.
 */
export interface UsersRoutesDeps {
  readonly principalRepository: PrincipalRepository;
  readonly clientAccessGrantRepository: ClientAccessGrantRepository;
  readonly anchorDomainRepository: AnchorDomainRepository;
  readonly clientAuthConfigRepository: ClientAuthConfigRepository;
  readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
  readonly createUserUseCase: UseCase<CreateUserCommand, UserCreated>;
  readonly updateUserUseCase: UseCase<UpdateUserCommand, UserUpdated>;
  readonly activateUserUseCase: UseCase<ActivateUserCommand, UserActivated>;
  readonly deactivateUserUseCase: UseCase<DeactivateUserCommand, UserDeactivated>;
  readonly deleteUserUseCase: UseCase<DeleteUserCommand, UserDeleted>;
  readonly assignRolesUseCase: UseCase<AssignRolesCommand, RolesAssigned>;
  readonly assignApplicationAccessUseCase: UseCase<
    AssignApplicationAccessCommand,
    ApplicationAccessAssigned
  >;
  readonly grantClientAccessUseCase: UseCase<GrantClientAccessCommand, ClientAccessGranted>;
  readonly revokeClientAccessUseCase: UseCase<RevokeClientAccessCommand, ClientAccessRevoked>;
}

/**
 * Register user admin API routes.
 */
export async function registerUsersRoutes(
  fastify: FastifyInstance,
  deps: UsersRoutesDeps,
): Promise<void> {
  const {
    principalRepository,
    clientAccessGrantRepository,
    anchorDomainRepository,
    clientAuthConfigRepository,
    applicationClientConfigRepository,
    createUserUseCase,
    updateUserUseCase,
    activateUserUseCase,
    deactivateUserUseCase,
    deleteUserUseCase,
    assignRolesUseCase,
    assignApplicationAccessUseCase,
    grantClientAccessUseCase,
    revokeClientAccessUseCase,
  } = deps;

  // POST /api/admin/users - Create user
  fastify.post(
    '/users',
    {
      schema: {
        body: CreateUserSchema,
        response: {
          201: UserResponseSchema,
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
    },
  );

  // GET /api/admin/users - List users
  fastify.get(
    '/users',
    {
      schema: {
        querystring: UsersListQuery,
        response: {
          200: UsersListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof UsersListQuery>;
      const page = parseInt(query.page ?? '0', 10);
      const pageSize = Math.min(parseInt(query.pageSize ?? '20', 10), 100);

      const pagedResult = await principalRepository.findPaged(page, pageSize);

      return jsonSuccess(reply, {
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
      });
    },
  );

  // GET /api/admin/users/:id - Get user by ID
  fastify.get(
    '/users/:id',
    {
      schema: {
        params: IdParam,
        response: {
          200: UserResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
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
    },
  );

  // PUT /api/admin/users/:id - Update user
  fastify.put(
    '/users/:id',
    {
      schema: {
        params: IdParam,
        body: UpdateUserSchema,
        response: {
          200: UserResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof UpdateUserSchema>;
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
    },
  );

  // POST /api/admin/users/:id/activate - Activate user
  fastify.post(
    '/users/:id/activate',
    {
      schema: {
        params: IdParam,
        response: {
          200: UserResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
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
    },
  );

  // POST /api/admin/users/:id/deactivate - Deactivate user
  fastify.post(
    '/users/:id/deactivate',
    {
      schema: {
        params: IdParam,
        response: {
          200: UserResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
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
    },
  );

  // DELETE /api/admin/users/:id - Delete user
  fastify.delete(
    '/users/:id',
    {
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

      const command: DeleteUserCommand = {
        userId: id,
      };

      const result = await deleteUserUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/users/:id/roles - Get user roles
  fastify.get(
    '/users/:id/roles',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: UserRolesResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const principal = await principalRepository.findById(id);

      if (!principal || principal.type !== 'USER') {
        return notFound(reply, `User not found: ${id}`);
      }

      return jsonSuccess(reply, {
        userId: principal.id,
        roles: principal.roles.map((r) => ({
          roleName: r.roleName,
          assignmentSource: r.assignmentSource,
          assignedAt: r.assignedAt.toISOString(),
        })),
      });
    },
  );

  // PUT /api/admin/users/:id/roles - Assign roles to user
  fastify.put(
    '/users/:id/roles',
    {
      preHandler: requirePermission(USER_PERMISSIONS.ASSIGN_ROLES),
      schema: {
        params: IdParam,
        body: AssignRolesSchema,
        response: {
          200: UserRolesResponseSchema,
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
            userId: principal.id,
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

  // GET /api/admin/users/:id/client-access - Get user client access grants
  fastify.get(
    '/users/:id/client-access',
    {
      preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: UserClientAccessResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const principal = await principalRepository.findById(id);

      if (!principal || principal.type !== 'USER') {
        return notFound(reply, `User not found: ${id}`);
      }

      const grants = await clientAccessGrantRepository.findByPrincipal(id);

      return jsonSuccess(reply, {
        userId: principal.id,
        grants: grants.map((g) => ({
          id: g.id,
          clientId: g.clientId,
          grantedBy: g.grantedBy,
          grantedAt: g.grantedAt.toISOString(),
        })),
      });
    },
  );

  // POST /api/admin/users/:id/client-access - Grant client access to user
  fastify.post(
    '/users/:id/client-access',
    {
      preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.GRANT),
      schema: {
        params: IdParam,
        body: GrantClientAccessSchema,
        response: {
          201: ClientAccessGrantResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof GrantClientAccessSchema>;
      const ctx = request.executionContext;

      const command: GrantClientAccessCommand = {
        userId: id,
        clientId: body.clientId,
      };

      const result = await grantClientAccessUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const grant = await clientAccessGrantRepository.findByPrincipalAndClient(id, body.clientId);
        if (grant) {
          return jsonCreated(reply, {
            id: grant.id,
            clientId: grant.clientId,
            grantedBy: grant.grantedBy,
            grantedAt: grant.grantedAt.toISOString(),
          });
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

  // GET /api/admin/users/check-email-domain - Check email domain configuration
  fastify.get(
    '/users/check-email-domain',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        querystring: EmailDomainCheckQuery,
        response: {
          200: EmailDomainCheckResponseSchema,
          400: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof EmailDomainCheckQuery>;
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
        info =
          'This email domain is configured as an anchor domain. Users will have platform-wide access.';
      } else if (hasAuthConfig && authConfig) {
        if (authConfig.authProvider === 'OIDC') {
          info = `This domain uses external OIDC authentication.`;
        } else {
          info = 'This domain has a configured authentication method.';
        }
      } else {
        info =
          'This domain will use internal authentication. Users will be created with a password.';
      }

      if (emailExists) {
        warning = 'A user with this email already exists.';
      }

      return jsonSuccess(reply, {
        domain,
        authProvider,
        isAnchorDomain,
        hasAuthConfig,
        emailExists,
        info,
        warning,
      });
    },
  );

  // GET /api/admin/users/:id/application-access - Get user application access
  fastify.get(
    '/users/:id/application-access',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: Type.Object({ applicationIds: Type.Array(Type.String()) }),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const principal = await principalRepository.findById(id);

      if (!principal || principal.type !== 'USER') {
        return notFound(reply, `User not found: ${id}`);
      }

      return jsonSuccess(reply, {
        applicationIds: [...principal.accessibleApplicationIds],
      });
    },
  );

  // PUT /api/admin/users/:id/application-access - Set user application access
  fastify.put(
    '/users/:id/application-access',
    {
      preHandler: requirePermission(USER_PERMISSIONS.ASSIGN_ROLES),
      schema: {
        params: IdParam,
        body: Type.Object({ applicationIds: Type.Array(Type.String()) }),
        response: {
          200: Type.Object({ applicationIds: Type.Array(Type.String()) }),
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as { applicationIds: string[] };
      const ctx = request.executionContext;

      const command: AssignApplicationAccessCommand = {
        userId: id,
        applicationIds: body.applicationIds,
      };

      const result = await assignApplicationAccessUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return jsonSuccess(reply, {
          applicationIds: body.applicationIds,
        });
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/users/:id/available-applications - Get apps enabled for user's clients
  fastify.get(
    '/users/:id/available-applications',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: Type.Object({
            applications: Type.Array(
              Type.Object({
                applicationId: Type.String(),
              }),
            ),
          }),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const principal = await principalRepository.findById(id);

      if (!principal || principal.type !== 'USER') {
        return notFound(reply, `User not found: ${id}`);
      }

      // Get client IDs the user can access
      const clientIds: string[] = [];

      // For CLIENT/PARTNER scope: use home client + client access grants
      if (principal.clientId) {
        clientIds.push(principal.clientId);
      }
      const grants = await clientAccessGrantRepository.findByPrincipal(id);
      for (const grant of grants) {
        if (!clientIds.includes(grant.clientId)) {
          clientIds.push(grant.clientId);
        }
      }

      // Find applications enabled for any of these clients
      const appIds = new Set<string>();
      for (const cid of clientIds) {
        const configs = await applicationClientConfigRepository.findByClient(cid);
        for (const config of configs) {
          appIds.add(config.applicationId);
        }
      }

      return jsonSuccess(reply, {
        applications: [...appIds].map((appId) => ({ applicationId: appId })),
      });
    },
  );
}
