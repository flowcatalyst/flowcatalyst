/**
 * Applications Admin API
 *
 * REST endpoints for application management.
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
  CreateApplicationCommand,
  UpdateApplicationCommand,
  DeleteApplicationCommand,
  EnableApplicationForClientCommand,
  DisableApplicationForClientCommand,
  ActivateApplicationCommand,
  DeactivateApplicationCommand,
  CreateServiceAccountCommand,
} from '../../application/index.js';
import type {
  ApplicationCreated,
  ApplicationUpdated,
  ApplicationDeleted,
  ApplicationEnabledForClient,
  ApplicationDisabledForClient,
  ApplicationActivated,
  ApplicationDeactivated,
  ApplicationType,
  ServiceAccountCreated,
} from '../../domain/index.js';
import type {
  ApplicationRepository,
  ApplicationClientConfigRepository,
  RoleRepository,
  PrincipalRepository,
} from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { APPLICATION_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateApplicationSchema = Type.Object({
  code: Type.String({ minLength: 1, maxLength: 50 }),
  name: Type.String({ minLength: 1, maxLength: 255 }),
  type: Type.Optional(Type.Union([Type.Literal('APPLICATION'), Type.Literal('INTEGRATION')])),
  description: Type.Optional(Type.Union([Type.String({ maxLength: 1000 }), Type.Null()])),
  iconUrl: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
  website: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
  logo: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  logoMimeType: Type.Optional(Type.Union([Type.String({ maxLength: 100 }), Type.Null()])),
  defaultBaseUrl: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
});

const UpdateApplicationSchema = Type.Object({
  name: Type.String({ minLength: 1, maxLength: 255 }),
  description: Type.Optional(Type.Union([Type.String({ maxLength: 1000 }), Type.Null()])),
  iconUrl: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
  website: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
  logo: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  logoMimeType: Type.Optional(Type.Union([Type.String({ maxLength: 100 }), Type.Null()])),
  defaultBaseUrl: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
});

const ClientIdSchema = Type.Object({
  clientId: Type.String({ minLength: 13, maxLength: 13 }),
});

const IdParam = Type.Object({ id: Type.String() });
const CodeParam = Type.Object({ code: Type.String() });
const IdClientIdParam = Type.Object({ id: Type.String(), clientId: Type.String() });

const ListApplicationsQuery = Type.Object({
  page: Type.Optional(Type.String()),
  pageSize: Type.Optional(Type.String()),
  type: Type.Optional(Type.String()),
});

type CreateApplicationBody = Static<typeof CreateApplicationSchema>;
type UpdateApplicationBody = Static<typeof UpdateApplicationSchema>;
type ClientIdBody = Static<typeof ClientIdSchema>;

// ─── Response Schemas ───────────────────────────────────────────────────────

const ApplicationResponseSchema = Type.Object({
  id: Type.String(),
  type: Type.String(),
  code: Type.String(),
  name: Type.String(),
  description: Type.Union([Type.String(), Type.Null()]),
  iconUrl: Type.Union([Type.String(), Type.Null()]),
  website: Type.Union([Type.String(), Type.Null()]),
  logo: Type.Union([Type.String(), Type.Null()]),
  logoMimeType: Type.Union([Type.String(), Type.Null()]),
  defaultBaseUrl: Type.Union([Type.String(), Type.Null()]),
  serviceAccountId: Type.Union([Type.String(), Type.Null()]),
  active: Type.Boolean(),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const ApplicationsListResponseSchema = Type.Object({
  applications: Type.Array(ApplicationResponseSchema),
  total: Type.Integer(),
  page: Type.Integer(),
  pageSize: Type.Integer(),
});

const ApplicationClientConfigResponseSchema = Type.Object({
  id: Type.String(),
  applicationId: Type.String(),
  clientId: Type.String(),
  enabled: Type.Boolean(),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const ApplicationClientConfigsListResponseSchema = Type.Object({
  configs: Type.Array(ApplicationClientConfigResponseSchema),
});

const ApplicationRolesResponseSchema = Type.Object({
  roles: Type.Array(
    Type.Object({
      id: Type.String(),
      name: Type.String(),
      displayName: Type.String(),
      description: Type.Union([Type.String(), Type.Null()]),
      permissions: Type.Array(Type.String()),
      clientManaged: Type.Boolean(),
    }),
  ),
});

const ProvisionServiceAccountResponseSchema = Type.Object({
  id: Type.String(),
  code: Type.String(),
  name: Type.String(),
  applicationId: Type.Union([Type.String(), Type.Null()]),
  active: Type.Boolean(),
  createdAt: Type.String({ format: 'date-time' }),
});

const ProvisionServiceAccountSchema = Type.Object({
  code: Type.Optional(Type.String({ minLength: 1, maxLength: 100 })),
  name: Type.Optional(Type.String({ minLength: 1, maxLength: 200 })),
});

type ApplicationResponse = Static<typeof ApplicationResponseSchema>;
type ApplicationClientConfigResponse = Static<typeof ApplicationClientConfigResponseSchema>;

/**
 * Dependencies for the applications API.
 */
export interface ApplicationsRoutesDeps {
  readonly applicationRepository: ApplicationRepository;
  readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
  readonly roleRepository: RoleRepository;
  readonly principalRepository: PrincipalRepository;
  readonly createApplicationUseCase: UseCase<CreateApplicationCommand, ApplicationCreated>;
  readonly updateApplicationUseCase: UseCase<UpdateApplicationCommand, ApplicationUpdated>;
  readonly deleteApplicationUseCase: UseCase<DeleteApplicationCommand, ApplicationDeleted>;
  readonly activateApplicationUseCase: UseCase<ActivateApplicationCommand, ApplicationActivated>;
  readonly deactivateApplicationUseCase: UseCase<
    DeactivateApplicationCommand,
    ApplicationDeactivated
  >;
  readonly enableApplicationForClientUseCase: UseCase<
    EnableApplicationForClientCommand,
    ApplicationEnabledForClient
  >;
  readonly disableApplicationForClientUseCase: UseCase<
    DisableApplicationForClientCommand,
    ApplicationDisabledForClient
  >;
  readonly createServiceAccountUseCase: UseCase<CreateServiceAccountCommand, ServiceAccountCreated>;
}

/**
 * Register application admin API routes.
 */
export async function registerApplicationsRoutes(
  fastify: FastifyInstance,
  deps: ApplicationsRoutesDeps,
): Promise<void> {
  const {
    applicationRepository,
    applicationClientConfigRepository,
    roleRepository,
    principalRepository,
    createApplicationUseCase,
    updateApplicationUseCase,
    deleteApplicationUseCase,
    activateApplicationUseCase,
    deactivateApplicationUseCase,
    enableApplicationForClientUseCase,
    disableApplicationForClientUseCase,
    createServiceAccountUseCase,
  } = deps;

  // POST /api/admin/applications - Create application
  fastify.post(
    '/applications',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.CREATE),
      schema: {
        body: CreateApplicationSchema,
        response: {
          201: ApplicationResponseSchema,
          400: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as CreateApplicationBody;
      const ctx = request.executionContext;

      const command: CreateApplicationCommand = {
        code: body.code,
        name: body.name,
        ...(body.type !== undefined && { type: body.type as ApplicationType }),
        description: body.description ?? null,
        iconUrl: body.iconUrl ?? null,
        website: body.website ?? null,
        logo: body.logo ?? null,
        logoMimeType: body.logoMimeType ?? null,
        defaultBaseUrl: body.defaultBaseUrl ?? null,
      };

      const result = await createApplicationUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const application = await applicationRepository.findById(
          result.value.getData().applicationId,
        );
        if (application) {
          return jsonCreated(reply, toApplicationResponse(application));
        }
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/applications - List applications
  fastify.get(
    '/applications',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.READ),
      schema: {
        querystring: ListApplicationsQuery,
        response: {
          200: ApplicationsListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof ListApplicationsQuery>;
      const page = parseInt(query.page ?? '0', 10);
      const pageSize = Math.min(parseInt(query.pageSize ?? '20', 10), 100);

      const pagedResult = await applicationRepository.findPaged(page, pageSize);

      return jsonSuccess(reply, {
        applications: pagedResult.items.map(toApplicationResponse),
        total: pagedResult.totalItems,
        page: pagedResult.page,
        pageSize: pagedResult.pageSize,
      });
    },
  );

  // GET /api/admin/applications/:id - Get application by ID
  fastify.get(
    '/applications/:id',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: ApplicationResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const application = await applicationRepository.findById(id);

      if (!application) {
        return notFound(reply, `Application not found: ${id}`);
      }

      return jsonSuccess(reply, toApplicationResponse(application));
    },
  );

  // GET /api/admin/applications/by-code/:code - Get application by code
  fastify.get(
    '/applications/by-code/:code',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.READ),
      schema: {
        params: CodeParam,
        response: {
          200: ApplicationResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { code } = request.params as Static<typeof CodeParam>;
      const application = await applicationRepository.findByCode(code);

      if (!application) {
        return notFound(reply, `Application not found with code: ${code}`);
      }

      return jsonSuccess(reply, toApplicationResponse(application));
    },
  );

  // PUT /api/admin/applications/:id - Update application
  fastify.put(
    '/applications/:id',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdateApplicationSchema,
        response: {
          200: ApplicationResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as UpdateApplicationBody;
      const ctx = request.executionContext;

      const command: UpdateApplicationCommand = {
        applicationId: id,
        name: body.name,
        description: body.description ?? null,
        iconUrl: body.iconUrl ?? null,
        website: body.website ?? null,
        logo: body.logo ?? null,
        logoMimeType: body.logoMimeType ?? null,
        defaultBaseUrl: body.defaultBaseUrl ?? null,
      };

      const result = await updateApplicationUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const application = await applicationRepository.findById(id);
        if (application) {
          return jsonSuccess(reply, toApplicationResponse(application));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/applications/:id/activate - Activate application
  fastify.post(
    '/applications/:id/activate',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.ACTIVATE),
      schema: {
        params: IdParam,
        response: {
          200: ApplicationResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: ActivateApplicationCommand = {
        applicationId: id,
      };

      const result = await activateApplicationUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const application = await applicationRepository.findById(id);
        if (application) {
          return jsonSuccess(reply, toApplicationResponse(application));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/applications/:id/deactivate - Deactivate application
  fastify.post(
    '/applications/:id/deactivate',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.DEACTIVATE),
      schema: {
        params: IdParam,
        response: {
          200: ApplicationResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: DeactivateApplicationCommand = {
        applicationId: id,
      };

      const result = await deactivateApplicationUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const application = await applicationRepository.findById(id);
        if (application) {
          return jsonSuccess(reply, toApplicationResponse(application));
        }
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/applications/:id - Delete application
  fastify.delete(
    '/applications/:id',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.DELETE),
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

      const command: DeleteApplicationCommand = {
        applicationId: id,
      };

      const result = await deleteApplicationUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/applications/:id/clients - Get client configs for application
  fastify.get(
    '/applications/:id/clients',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: ApplicationClientConfigsListResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;

      // Verify application exists
      const applicationExists = await applicationRepository.exists(id);
      if (!applicationExists) {
        return notFound(reply, `Application not found: ${id}`);
      }

      const configs = await applicationClientConfigRepository.findByApplication(id);

      return jsonSuccess(reply, {
        configs: configs.map(toApplicationClientConfigResponse),
      });
    },
  );

  // POST /api/admin/applications/:id/clients - Enable application for client
  fastify.post(
    '/applications/:id/clients',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.ENABLE_CLIENT),
      schema: {
        params: IdParam,
        body: ClientIdSchema,
        response: {
          201: ApplicationClientConfigResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as ClientIdBody;
      const ctx = request.executionContext;

      const command: EnableApplicationForClientCommand = {
        applicationId: id,
        clientId: body.clientId,
      };

      const result = await enableApplicationForClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const config = await applicationClientConfigRepository.findByApplicationAndClient(
          id,
          body.clientId,
        );
        if (config) {
          return jsonCreated(reply, toApplicationClientConfigResponse(config));
        }
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/applications/:id/clients/:clientId - Disable application for client
  fastify.delete(
    '/applications/:id/clients/:clientId',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.DISABLE_CLIENT),
      schema: {
        params: IdClientIdParam,
        response: {
          204: Type.Null(),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id, clientId } = request.params as Static<typeof IdClientIdParam>;
      const ctx = request.executionContext;

      const command: DisableApplicationForClientCommand = {
        applicationId: id,
        clientId,
      };

      const result = await disableApplicationForClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/applications/:id/roles - Get roles for application (Java parity)
  fastify.get(
    '/applications/:id/roles',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: ApplicationRolesResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;

      const application = await applicationRepository.findById(id);
      if (!application) {
        return notFound(reply, `Application not found: ${id}`);
      }

      const roles = await roleRepository.findByApplicationId(id);

      return jsonSuccess(reply, {
        roles: roles.map((r) => ({
          id: r.id,
          name: r.name,
          displayName: r.displayName,
          description: r.description,
          permissions: [...r.permissions],
          clientManaged: r.clientManaged,
        })),
      });
    },
  );

  // POST /api/admin/applications/:id/provision-service-account - Provision service account (Java parity)
  fastify.post(
    '/applications/:id/provision-service-account',
    {
      preHandler: requirePermission(APPLICATION_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: ProvisionServiceAccountSchema,
        response: {
          201: ProvisionServiceAccountResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof ProvisionServiceAccountSchema>;
      const ctx = request.executionContext;

      const application = await applicationRepository.findById(id);
      if (!application) {
        return notFound(reply, `Application not found: ${id}`);
      }

      // Check if application already has a service account
      if (application.serviceAccountId) {
        const existing = await principalRepository.findById(application.serviceAccountId);
        if (existing && existing.serviceAccount) {
          return jsonSuccess(reply, {
            id: existing.id,
            code: existing.serviceAccount.code,
            name: existing.name,
            applicationId: existing.applicationId,
            active: existing.active,
            createdAt: existing.createdAt.toISOString(),
          });
        }
      }

      const command: CreateServiceAccountCommand = {
        code: body.code ?? `${application.code}-service`,
        name: body.name ?? `${application.name} Service Account`,
        description: `Auto-provisioned service account for ${application.name}`,
        applicationId: id,
        clientId: null,
      };

      const result = await createServiceAccountUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(result.value.getData().principalId);
        if (principal) {
          return jsonCreated(reply, {
            id: principal.id,
            code: principal.serviceAccount?.code ?? command.code,
            name: principal.name,
            applicationId: principal.applicationId,
            active: principal.active,
            createdAt: principal.createdAt.toISOString(),
          });
        }
      }

      return sendResult(reply, result);
    },
  );
}

/**
 * Convert an Application entity to an ApplicationResponse.
 */
function toApplicationResponse(application: {
  id: string;
  type: string;
  code: string;
  name: string;
  description: string | null;
  iconUrl: string | null;
  website: string | null;
  logo: string | null;
  logoMimeType: string | null;
  defaultBaseUrl: string | null;
  serviceAccountId: string | null;
  active: boolean;
  createdAt: Date;
  updatedAt: Date;
}): ApplicationResponse {
  return {
    id: application.id,
    type: application.type,
    code: application.code,
    name: application.name,
    description: application.description,
    iconUrl: application.iconUrl,
    website: application.website,
    logo: application.logo,
    logoMimeType: application.logoMimeType,
    defaultBaseUrl: application.defaultBaseUrl,
    serviceAccountId: application.serviceAccountId,
    active: application.active,
    createdAt: application.createdAt.toISOString(),
    updatedAt: application.updatedAt.toISOString(),
  };
}

/**
 * Convert an ApplicationClientConfig entity to an ApplicationClientConfigResponse.
 */
function toApplicationClientConfigResponse(config: {
  id: string;
  applicationId: string;
  clientId: string;
  enabled: boolean;
  createdAt: Date;
  updatedAt: Date;
}): ApplicationClientConfigResponse {
  return {
    id: config.id,
    applicationId: config.applicationId,
    clientId: config.clientId,
    enabled: config.enabled,
    createdAt: config.createdAt.toISOString(),
    updatedAt: config.updatedAt.toISOString(),
  };
}
