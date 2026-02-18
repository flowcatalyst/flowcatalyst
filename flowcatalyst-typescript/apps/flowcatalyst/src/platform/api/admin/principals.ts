/**
 * Principals Admin API
 *
 * REST endpoints for principal (user + service account) management.
 * Matches the Java PrincipalAdminResource contract.
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
import type { PasswordService } from '@flowcatalyst/platform-crypto';

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
  Principal,
} from '../../domain/index.js';
import type { ApplicationRepository } from '../../infrastructure/persistence/repositories/application-repository.js';
import type { PrincipalRepository } from '../../infrastructure/persistence/repositories/principal-repository.js';
import type { ClientAccessGrantRepository } from '../../infrastructure/persistence/repositories/client-access-grant-repository.js';
import type { AnchorDomainRepository } from '../../infrastructure/persistence/repositories/anchor-domain-repository.js';
import type { ClientAuthConfigRepository } from '../../infrastructure/persistence/repositories/client-auth-config-repository.js';
import type { ApplicationClientConfigRepository } from '../../infrastructure/persistence/index.js';
import type { ClientRepository } from '../../infrastructure/persistence/repositories/client-repository.js';
import type { EmailDomainMappingRepository } from '../../infrastructure/persistence/repositories/email-domain-mapping-repository.js';
import type { IdentityProviderRepository } from '../../infrastructure/persistence/repositories/identity-provider-repository.js';
import { requirePermission } from '../../authorization/index.js';
import {
  USER_PERMISSIONS,
  CLIENT_ACCESS_PERMISSIONS,
} from '../../authorization/permissions/platform-iam.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateUserSchema = Type.Object({
  email: Type.String({ format: 'email' }),
  password: Type.Optional(Type.Union([Type.String({ minLength: 12 }), Type.Null()])),
  name: Type.String({ minLength: 1 }),
  clientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const UpdatePrincipalSchema = Type.Object({
  name: Type.String({ minLength: 1 }),
});

const AssignRolesSchema = Type.Object({
  roles: Type.Array(Type.String()),
});

const AssignRoleSchema = Type.Object({
  roleName: Type.String({ minLength: 1 }),
});

const GrantClientAccessSchema = Type.Object({
  clientId: Type.String(),
});

const ResetPasswordSchema = Type.Object({
  newPassword: Type.String({ minLength: 12 }),
});

const AssignApplicationAccessSchema = Type.Object({
  applicationIds: Type.Array(Type.String()),
});

// ─── Param Schemas ──────────────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });
const IdClientParam = Type.Object({ id: Type.String(), clientId: Type.String() });
const IdRoleParam = Type.Object({ id: Type.String(), roleName: Type.String() });

// ─── Query Schemas ──────────────────────────────────────────────────────────

const PrincipalsListQuery = Type.Object({
  type: Type.Optional(Type.String()),
  clientId: Type.Optional(Type.String()),
  active: Type.Optional(Type.String()),
  email: Type.Optional(Type.String()),
  q: Type.Optional(Type.String()),
  page: Type.Optional(Type.String()),
  pageSize: Type.Optional(Type.String()),
});

const EmailDomainCheckQuery = Type.Object({
  email: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const PrincipalResponseSchema = Type.Object({
  id: Type.String(),
  type: Type.String(),
  scope: Type.Union([Type.String(), Type.Null()]),
  clientId: Type.Union([Type.String(), Type.Null()]),
  name: Type.String(),
  active: Type.Boolean(),
  email: Type.Union([Type.String(), Type.Null()]),
  idpType: Type.Union([Type.String(), Type.Null()]),
  roles: Type.Array(Type.String()),
  isAnchorUser: Type.Boolean(),
  grantedClientIds: Type.Array(Type.String()),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const PrincipalDetailResponseSchema = Type.Object({
  id: Type.String(),
  type: Type.String(),
  scope: Type.Union([Type.String(), Type.Null()]),
  clientId: Type.Union([Type.String(), Type.Null()]),
  name: Type.String(),
  active: Type.Boolean(),
  email: Type.Union([Type.String(), Type.Null()]),
  idpType: Type.Union([Type.String(), Type.Null()]),
  roles: Type.Array(Type.String()),
  isAnchorUser: Type.Boolean(),
  grantedClientIds: Type.Array(Type.String()),
  lastLoginAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const PrincipalListResponseSchema = Type.Object({
  principals: Type.Array(PrincipalResponseSchema),
  total: Type.Integer(),
});

const RoleAssignmentResponseSchema = Type.Object({
  roleName: Type.String(),
  assignmentSource: Type.String(),
  assignedAt: Type.String({ format: 'date-time' }),
});

const RoleListResponseSchema = Type.Object({
  roles: Type.Array(RoleAssignmentResponseSchema),
});

const RolesAssignedResponseSchema = Type.Object({
  roles: Type.Array(RoleAssignmentResponseSchema),
  added: Type.Array(Type.String()),
  removed: Type.Array(Type.String()),
});

const ClientAccessGrantResponseSchema = Type.Object({
  id: Type.String(),
  clientId: Type.String(),
  grantedBy: Type.String(),
  grantedAt: Type.String({ format: 'date-time' }),
});

const ClientAccessListResponseSchema = Type.Object({
  grants: Type.Array(ClientAccessGrantResponseSchema),
});

const StatusResponseSchema = Type.Object({
  message: Type.String(),
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

const ApplicationAccessItemSchema = Type.Object({
  id: Type.String(),
  code: Type.String(),
  name: Type.String(),
  grantedAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
});

const ApplicationAccessResponseSchema = Type.Object({
  applications: Type.Array(ApplicationAccessItemSchema),
});

const ApplicationAccessAssignedResponseSchema = Type.Object({
  applications: Type.Array(ApplicationAccessItemSchema),
  added: Type.Array(Type.String()),
  removed: Type.Array(Type.String()),
});

type PrincipalResponse = Static<typeof PrincipalResponseSchema>;

/**
 * Map a Principal domain object to a PrincipalDto response.
 */
function toPrincipalDto(principal: Principal, grantedClientIds: string[]): PrincipalResponse {
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
    isAnchorUser: principal.scope === 'ANCHOR',
    grantedClientIds,
    createdAt: principal.createdAt.toISOString(),
    updatedAt: principal.updatedAt.toISOString(),
  };
}

/**
 * Dependencies for the principals API.
 */
export interface PrincipalsRoutesDeps {
  readonly principalRepository: PrincipalRepository;
  readonly applicationRepository: ApplicationRepository;
  readonly clientRepository: ClientRepository;
  readonly clientAccessGrantRepository: ClientAccessGrantRepository;
  readonly anchorDomainRepository: AnchorDomainRepository;
  readonly clientAuthConfigRepository: ClientAuthConfigRepository;
  readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
  readonly emailDomainMappingRepository: EmailDomainMappingRepository;
  readonly identityProviderRepository: IdentityProviderRepository;
  readonly passwordService: PasswordService;
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
 * Register principal admin API routes.
 */
export async function registerPrincipalsRoutes(
  fastify: FastifyInstance,
  deps: PrincipalsRoutesDeps,
): Promise<void> {
  const {
    principalRepository,
    applicationRepository,
    clientRepository,
    clientAccessGrantRepository,
    anchorDomainRepository,
    clientAuthConfigRepository,
    applicationClientConfigRepository,
    emailDomainMappingRepository,
    identityProviderRepository,
    passwordService,
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

  // GET /api/admin/principals - List principals with filters
  fastify.get(
    '/principals',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        querystring: PrincipalsListQuery,
        response: {
          200: PrincipalListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof PrincipalsListQuery>;
      const page = parseInt(query.page ?? '0', 10);
      const pageSize = Math.min(parseInt(query.pageSize ?? '20', 10), 100);

      const filters = {
        type: query.type as 'USER' | 'SERVICE' | undefined,
        clientId: query.clientId,
        active: query.active !== undefined ? query.active === 'true' : undefined,
        email: query.email,
        search: query.q,
      };

      const pagedResult = await principalRepository.findFilteredPaged(filters, page, pageSize);

      // Batch load grantedClientIds
      const ids = pagedResult.items.map((p) => p.id);
      const grantsMap = await clientAccessGrantRepository.findByPrincipalIds(ids);

      return jsonSuccess(reply, {
        principals: pagedResult.items.map((p) => toPrincipalDto(p, grantsMap.get(p.id) ?? [])),
        total: pagedResult.totalItems,
      });
    },
  );

  // GET /api/admin/principals/:id - Get principal by ID
  fastify.get(
    '/principals/:id',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: PrincipalDetailResponseSchema,
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
      const grantedClientIds = grants.map((g) => g.clientId);

      return jsonSuccess(reply, {
        id: principal.id,
        type: principal.type,
        scope: principal.scope,
        clientId: principal.clientId,
        name: principal.name,
        active: principal.active,
        email: principal.userIdentity?.email ?? null,
        idpType: principal.userIdentity?.idpType ?? null,
        roles: principal.roles.map((r) => r.roleName),
        isAnchorUser: principal.scope === 'ANCHOR',
        grantedClientIds,
        lastLoginAt: principal.userIdentity?.lastLoginAt?.toISOString() ?? null,
        createdAt: principal.createdAt.toISOString(),
        updatedAt: principal.updatedAt.toISOString(),
      });
    },
  );

  // POST /api/admin/principals/users - Create user
  fastify.post(
    '/principals/users',
    {
      preHandler: requirePermission(USER_PERMISSIONS.CREATE),
      schema: {
        body: CreateUserSchema,
        response: {
          201: PrincipalResponseSchema,
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
        const response: PrincipalResponse = {
          id: event.getData().userId,
          type: 'USER',
          scope: event.getData().scope,
          clientId: event.getData().clientId,
          name: event.getData().name,
          active: true,
          email: event.getData().email,
          idpType: event.getData().idpType,
          roles: [],
          isAnchorUser: event.getData().scope === 'ANCHOR',
          grantedClientIds: [],
          createdAt: event.time.toISOString(),
          updatedAt: event.time.toISOString(),
        };
        return jsonCreated(reply, response);
      }

      return sendResult(reply, result);
    },
  );

  // PUT /api/admin/principals/:id - Update principal
  fastify.put(
    '/principals/:id',
    {
      preHandler: requirePermission(USER_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdatePrincipalSchema,
        response: {
          200: PrincipalResponseSchema,
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
      };

      const result = await updateUserUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(id);
        if (principal) {
          const grants = await clientAccessGrantRepository.findByPrincipal(id);
          return jsonSuccess(
            reply,
            toPrincipalDto(
              principal,
              grants.map((g) => g.clientId),
            ),
          );
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/principals/:id/activate - Activate principal
  fastify.post(
    '/principals/:id/activate',
    {
      preHandler: requirePermission(USER_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        response: {
          200: StatusResponseSchema,
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
        return jsonSuccess(reply, { message: 'Principal activated' });
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/principals/:id/deactivate - Deactivate principal
  fastify.post(
    '/principals/:id/deactivate',
    {
      preHandler: requirePermission(USER_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        response: {
          200: StatusResponseSchema,
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
        return jsonSuccess(reply, { message: 'Principal deactivated' });
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/principals/:id - Delete principal
  fastify.delete(
    '/principals/:id',
    {
      preHandler: requirePermission(USER_PERMISSIONS.DELETE),
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

      const command: DeleteUserCommand = { userId: id };
      const result = await deleteUserUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/principals/:id/reset-password - Reset password
  fastify.post(
    '/principals/:id/reset-password',
    {
      preHandler: requirePermission(USER_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: ResetPasswordSchema,
        response: {
          200: StatusResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof ResetPasswordSchema>;

      const principal = await principalRepository.findById(id);
      if (!principal || principal.type !== 'USER') {
        return notFound(reply, `User not found: ${id}`);
      }

      if (!principal.userIdentity || principal.userIdentity.idpType !== 'INTERNAL') {
        return badRequest(
          reply,
          'Password reset is only supported for internal authentication users',
        );
      }

      const hashResult = await passwordService.validateAndHash(body.newPassword);
      let hash: string;
      if (hashResult.isOk()) {
        hash = hashResult.value;
      } else {
        return badRequest(reply, 'Password does not meet complexity requirements');
      }

      // Update the principal's password hash directly
      const updated: Principal = {
        ...principal,
        userIdentity: {
          ...principal.userIdentity,
          passwordHash: hash,
        },
      };
      await principalRepository.update(updated);

      return jsonSuccess(reply, { message: 'Password reset successfully' });
    },
  );

  // GET /api/admin/principals/:id/roles - Get principal roles
  fastify.get(
    '/principals/:id/roles',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: RoleListResponseSchema,
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

  // POST /api/admin/principals/:id/roles - Assign single role
  fastify.post(
    '/principals/:id/roles',
    {
      preHandler: requirePermission(USER_PERMISSIONS.ASSIGN_ROLES),
      schema: {
        params: IdParam,
        body: AssignRoleSchema,
        response: {
          201: RoleAssignmentResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof AssignRoleSchema>;
      const ctx = request.executionContext;

      const principal = await principalRepository.findById(id);
      if (!principal) {
        return notFound(reply, `Principal not found: ${id}`);
      }

      // Combine existing roles with the new one
      const existingRoleNames = principal.roles.map((r) => r.roleName);
      if (existingRoleNames.includes(body.roleName)) {
        return badRequest(reply, `Role already assigned: ${body.roleName}`);
      }

      const command: AssignRolesCommand = {
        userId: id,
        roles: [...existingRoleNames, body.roleName],
      };

      const result = await assignRolesUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const updated = await principalRepository.findById(id);
        const assignment = updated?.roles.find((r) => r.roleName === body.roleName);
        if (assignment) {
          return jsonCreated(reply, {
            roleName: assignment.roleName,
            assignmentSource: assignment.assignmentSource,
            assignedAt: assignment.assignedAt.toISOString(),
          });
        }
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/principals/:id/roles/:roleName - Remove single role
  fastify.delete(
    '/principals/:id/roles/:roleName',
    {
      preHandler: requirePermission(USER_PERMISSIONS.ASSIGN_ROLES),
      schema: {
        params: IdRoleParam,
        response: {
          204: Type.Null(),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id, roleName } = request.params as Static<typeof IdRoleParam>;
      const ctx = request.executionContext;

      const principal = await principalRepository.findById(id);
      if (!principal) {
        return notFound(reply, `Principal not found: ${id}`);
      }

      const remainingRoles = principal.roles
        .map((r) => r.roleName)
        .filter((name) => name !== roleName);

      const command: AssignRolesCommand = {
        userId: id,
        roles: remainingRoles,
      };

      const result = await assignRolesUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // PUT /api/admin/principals/:id/roles - Declarative batch assign roles
  fastify.put(
    '/principals/:id/roles',
    {
      preHandler: requirePermission(USER_PERMISSIONS.ASSIGN_ROLES),
      schema: {
        params: IdParam,
        body: AssignRolesSchema,
        response: {
          200: RolesAssignedResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof AssignRolesSchema>;
      const ctx = request.executionContext;

      const principal = await principalRepository.findById(id);
      if (!principal) {
        return notFound(reply, `Principal not found: ${id}`);
      }

      const previousRoles = new Set(principal.roles.map((r) => r.roleName));
      const newRoles = new Set(body.roles);

      const command: AssignRolesCommand = {
        userId: id,
        roles: body.roles,
      };

      const result = await assignRolesUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const updated = await principalRepository.findById(id);
        if (updated) {
          const added = body.roles.filter((r) => !previousRoles.has(r));
          const removed = [...previousRoles].filter((r) => !newRoles.has(r));

          return jsonSuccess(reply, {
            roles: updated.roles.map((r) => ({
              roleName: r.roleName,
              assignmentSource: r.assignmentSource,
              assignedAt: r.assignedAt.toISOString(),
            })),
            added,
            removed,
          });
        }
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/principals/:id/client-access - Get client access grants
  fastify.get(
    '/principals/:id/client-access',
    {
      preHandler: requirePermission(CLIENT_ACCESS_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: ClientAccessListResponseSchema,
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
          grantedBy: g.grantedBy,
          grantedAt: g.grantedAt.toISOString(),
        })),
      });
    },
  );

  // POST /api/admin/principals/:id/client-access - Grant client access
  fastify.post(
    '/principals/:id/client-access',
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

  // DELETE /api/admin/principals/:id/client-access/:clientId - Revoke client access
  fastify.delete(
    '/principals/:id/client-access/:clientId',
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

  // GET /api/admin/principals/check-email-domain - Check email domain configuration
  fastify.get(
    '/principals/check-email-domain',
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

      const atIndex = email.indexOf('@');
      if (atIndex === -1 || atIndex === 0 || atIndex === email.length - 1) {
        return badRequest(reply, 'Invalid email format');
      }
      const domain = email.substring(atIndex + 1).toLowerCase();
      if (!domain || domain.indexOf('.') === -1) {
        return badRequest(reply, 'Invalid email domain');
      }

      const emailExists = await principalRepository.existsByEmail(email);
      const isAnchorDomain = await anchorDomainRepository.existsByDomain(domain);

      // Check email domain mappings (linked to identity providers)
      const domainMapping = await emailDomainMappingRepository.findByEmailDomain(domain);
      let authProvider = 'INTERNAL';
      let hasAuthConfig = false;

      if (domainMapping) {
        hasAuthConfig = true;
        const idp = await identityProviderRepository.findById(domainMapping.identityProviderId);
        if (idp) {
          authProvider = idp.type; // 'INTERNAL' or 'OIDC'
        }
      } else {
        // Fall back to legacy client auth config
        const authConfig = await clientAuthConfigRepository.findByEmailDomain(domain);
        if (authConfig) {
          hasAuthConfig = true;
          authProvider = authConfig.authProvider;
        }
      }

      let info: string | null = null;
      let warning: string | null = null;

      if (isAnchorDomain) {
        info =
          'This email domain is configured as an anchor domain. Users will have platform-wide access.';
      } else if (hasAuthConfig) {
        if (authProvider === 'OIDC') {
          info = 'This domain uses external OIDC authentication.';
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

  // GET /api/admin/principals/:id/application-access - Get application access
  fastify.get(
    '/principals/:id/application-access',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: ApplicationAccessResponseSchema,
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

      const accessRecords = await principalRepository.loadApplicationAccessWithGrantedAt(id);
      const appIds = accessRecords.map((r) => r.applicationId);
      const apps = appIds.length > 0 ? await applicationRepository.findByIds(appIds) : [];
      const appsById = new Map(apps.map((a) => [a.id, a]));

      return jsonSuccess(reply, {
        applications: accessRecords.map((r) => {
          const app = appsById.get(r.applicationId);
          return {
            id: r.applicationId,
            code: app?.code ?? '',
            name: app?.name ?? '',
            grantedAt: r.grantedAt.toISOString(),
          };
        }),
      });
    },
  );

  // PUT /api/admin/principals/:id/application-access - Set application access (declarative)
  fastify.put(
    '/principals/:id/application-access',
    {
      preHandler: requirePermission(USER_PERMISSIONS.ASSIGN_ROLES),
      schema: {
        params: IdParam,
        body: AssignApplicationAccessSchema,
        response: {
          200: ApplicationAccessAssignedResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof AssignApplicationAccessSchema>;
      const ctx = request.executionContext;

      const principal = await principalRepository.findById(id);
      if (!principal) {
        return notFound(reply, `Principal not found: ${id}`);
      }

      const previousAppIds = new Set(principal.accessibleApplicationIds);
      const newAppIds = new Set(body.applicationIds);

      const command: AssignApplicationAccessCommand = {
        userId: id,
        applicationIds: body.applicationIds,
      };

      const result = await assignApplicationAccessUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const added = body.applicationIds.filter((id) => !previousAppIds.has(id));
        const removed = [...previousAppIds].filter((id) => !newAppIds.has(id));

        const accessRecords = await principalRepository.loadApplicationAccessWithGrantedAt(id);
        const appIds = accessRecords.map((r) => r.applicationId);
        const apps = appIds.length > 0 ? await applicationRepository.findByIds(appIds) : [];
        const appsById = new Map(apps.map((a) => [a.id, a]));

        return jsonSuccess(reply, {
          applications: accessRecords.map((r) => {
            const app = appsById.get(r.applicationId);
            return {
              id: r.applicationId,
              code: app?.code ?? '',
              name: app?.name ?? '',
              grantedAt: r.grantedAt.toISOString(),
            };
          }),
          added,
          removed,
        });
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/principals/:id/available-applications - Get apps available to a principal
  fastify.get(
    '/principals/:id/available-applications',
    {
      preHandler: requirePermission(USER_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: ApplicationAccessResponseSchema,
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

      // ANCHOR users have access to ALL applications
      if (principal.scope === 'ANCHOR') {
        const allApps = await applicationRepository.findAll();
        return jsonSuccess(reply, {
          applications: allApps.map((app) => ({
            id: app.id,
            code: app.code,
            name: app.name,
            grantedAt: null,
          })),
        });
      }

      // For non-ANCHOR users, determine accessible apps through client configs
      const clientIds: string[] = [];
      if (principal.clientId) {
        clientIds.push(principal.clientId);
      }
      const grants = await clientAccessGrantRepository.findByPrincipal(id);
      for (const grant of grants) {
        if (!clientIds.includes(grant.clientId)) {
          clientIds.push(grant.clientId);
        }
      }

      const appIdSet = new Set<string>();
      for (const cid of clientIds) {
        const configs = await applicationClientConfigRepository.findByClient(cid);
        for (const config of configs) {
          appIdSet.add(config.applicationId);
        }
      }

      const appIds = [...appIdSet];
      const apps = appIds.length > 0 ? await applicationRepository.findByIds(appIds) : [];

      return jsonSuccess(reply, {
        applications: apps.map((app) => ({
          id: app.id,
          code: app.code,
          name: app.name,
          grantedAt: null,
        })),
      });
    },
  );
}
