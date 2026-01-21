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
	badRequest,
	safeValidate,
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
} from '../../domain/index.js';
import type {
	ApplicationRepository,
	ApplicationClientConfigRepository,
} from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { APPLICATION_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// Request schemas using TypeBox
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

type CreateApplicationBody = Static<typeof CreateApplicationSchema>;
type UpdateApplicationBody = Static<typeof UpdateApplicationSchema>;
type ClientIdBody = Static<typeof ClientIdSchema>;

// Response schemas
interface ApplicationResponse {
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
	createdAt: string;
	updatedAt: string;
}

interface ApplicationsListResponse {
	applications: ApplicationResponse[];
	total: number;
	page: number;
	pageSize: number;
}

interface ApplicationClientConfigResponse {
	id: string;
	applicationId: string;
	clientId: string;
	enabled: boolean;
	createdAt: string;
	updatedAt: string;
}

/**
 * Dependencies for the applications API.
 */
export interface ApplicationsRoutesDeps {
	readonly applicationRepository: ApplicationRepository;
	readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
	readonly createApplicationUseCase: UseCase<CreateApplicationCommand, ApplicationCreated>;
	readonly updateApplicationUseCase: UseCase<UpdateApplicationCommand, ApplicationUpdated>;
	readonly deleteApplicationUseCase: UseCase<DeleteApplicationCommand, ApplicationDeleted>;
	readonly activateApplicationUseCase: UseCase<ActivateApplicationCommand, ApplicationActivated>;
	readonly deactivateApplicationUseCase: UseCase<DeactivateApplicationCommand, ApplicationDeactivated>;
	readonly enableApplicationForClientUseCase: UseCase<EnableApplicationForClientCommand, ApplicationEnabledForClient>;
	readonly disableApplicationForClientUseCase: UseCase<
		DisableApplicationForClientCommand,
		ApplicationDisabledForClient
	>;
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
		createApplicationUseCase,
		updateApplicationUseCase,
		deleteApplicationUseCase,
		activateApplicationUseCase,
		deactivateApplicationUseCase,
		enableApplicationForClientUseCase,
		disableApplicationForClientUseCase,
	} = deps;

	// POST /api/admin/applications - Create application
	fastify.post(
		'/applications',
		{
			preHandler: requirePermission(APPLICATION_PERMISSIONS.CREATE),
		},
		async (request, reply) => {
			const bodyResult = safeValidate(request.body, CreateApplicationSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as CreateApplicationBody;
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
				const application = await applicationRepository.findById(result.value.getData().applicationId);
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
		},
		async (request, reply) => {
			const query = request.query as { page?: string; pageSize?: string; type?: string };
			const page = parseInt(query.page ?? '0', 10);
			const pageSize = Math.min(parseInt(query.pageSize ?? '20', 10), 100);

			const pagedResult = await applicationRepository.findPaged(page, pageSize);

			const response: ApplicationsListResponse = {
				applications: pagedResult.items.map(toApplicationResponse),
				total: pagedResult.totalItems,
				page: pagedResult.page,
				pageSize: pagedResult.pageSize,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/applications/:id - Get application by ID
	fastify.get(
		'/applications/:id',
		{
			preHandler: requirePermission(APPLICATION_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
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
		},
		async (request, reply) => {
			const { code } = request.params as { code: string };
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
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateApplicationSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateApplicationBody;
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
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
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
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
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
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
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
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };

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
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, ClientIdSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as ClientIdBody;
			const ctx = request.executionContext;

			const command: EnableApplicationForClientCommand = {
				applicationId: id,
				clientId: body.clientId,
			};

			const result = await enableApplicationForClientUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const config = await applicationClientConfigRepository.findByApplicationAndClient(id, body.clientId);
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
		},
		async (request, reply) => {
			const { id, clientId } = request.params as { id: string; clientId: string };
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
