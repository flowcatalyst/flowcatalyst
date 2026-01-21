/**
 * Auth Configs Admin API
 *
 * REST endpoints for client authentication configuration management.
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
	CreateInternalAuthConfigCommand,
	CreateOidcAuthConfigCommand,
	UpdateOidcSettingsCommand,
	UpdateConfigTypeCommand,
	UpdateAdditionalClientsCommand,
	UpdateGrantedClientsCommand,
	DeleteAuthConfigCommand,
} from '../../application/index.js';
import type {
	AuthConfigCreated,
	AuthConfigUpdated,
	AuthConfigDeleted,
	ClientAuthConfig,
	AuthConfigType,
	AuthProvider,
} from '../../domain/index.js';
import type { ClientAuthConfigRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { AUTH_CONFIG_PERMISSIONS } from '../../authorization/permissions/platform-iam.js';

// Request schemas using TypeBox
const CreateInternalAuthConfigSchema = Type.Object({
	emailDomain: Type.String({ minLength: 1 }),
	configType: Type.Union([Type.Literal('ANCHOR'), Type.Literal('PARTNER'), Type.Literal('CLIENT')]),
	primaryClientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	additionalClientIds: Type.Optional(Type.Array(Type.String())),
	grantedClientIds: Type.Optional(Type.Array(Type.String())),
});

const CreateOidcAuthConfigSchema = Type.Object({
	emailDomain: Type.String({ minLength: 1 }),
	configType: Type.Union([Type.Literal('ANCHOR'), Type.Literal('PARTNER'), Type.Literal('CLIENT')]),
	primaryClientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	additionalClientIds: Type.Optional(Type.Array(Type.String())),
	grantedClientIds: Type.Optional(Type.Array(Type.String())),
	oidcIssuerUrl: Type.String({ minLength: 1 }),
	oidcClientId: Type.String({ minLength: 1 }),
	oidcClientSecretRef: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	oidcMultiTenant: Type.Optional(Type.Boolean()),
	oidcIssuerPattern: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const UpdateOidcSettingsSchema = Type.Object({
	oidcIssuerUrl: Type.String({ minLength: 1 }),
	oidcClientId: Type.String({ minLength: 1 }),
	oidcClientSecretRef: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	oidcMultiTenant: Type.Optional(Type.Boolean()),
	oidcIssuerPattern: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const UpdateConfigTypeSchema = Type.Object({
	configType: Type.Union([Type.Literal('ANCHOR'), Type.Literal('PARTNER'), Type.Literal('CLIENT')]),
	primaryClientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const UpdateAdditionalClientsSchema = Type.Object({
	clientIds: Type.Array(Type.String()),
});

const UpdateGrantedClientsSchema = Type.Object({
	clientIds: Type.Array(Type.String()),
});

type CreateInternalAuthConfigBody = Static<typeof CreateInternalAuthConfigSchema>;
type CreateOidcAuthConfigBody = Static<typeof CreateOidcAuthConfigSchema>;
type UpdateOidcSettingsBody = Static<typeof UpdateOidcSettingsSchema>;
type UpdateConfigTypeBody = Static<typeof UpdateConfigTypeSchema>;
type UpdateAdditionalClientsBody = Static<typeof UpdateAdditionalClientsSchema>;
type UpdateGrantedClientsBody = Static<typeof UpdateGrantedClientsSchema>;

// Response schemas
interface AuthConfigResponse {
	id: string;
	emailDomain: string;
	configType: AuthConfigType;
	primaryClientId: string | null;
	additionalClientIds: string[];
	grantedClientIds: string[];
	authProvider: AuthProvider;
	oidcIssuerUrl: string | null;
	oidcClientId: string | null;
	oidcMultiTenant: boolean;
	oidcIssuerPattern: string | null;
	hasClientSecret: boolean;
	createdAt: string;
	updatedAt: string;
}

interface AuthConfigListResponse {
	configs: AuthConfigResponse[];
	total: number;
}

/**
 * Dependencies for the auth configs API.
 */
export interface AuthConfigsRoutesDeps {
	readonly clientAuthConfigRepository: ClientAuthConfigRepository;
	readonly createInternalAuthConfigUseCase: UseCase<CreateInternalAuthConfigCommand, AuthConfigCreated>;
	readonly createOidcAuthConfigUseCase: UseCase<CreateOidcAuthConfigCommand, AuthConfigCreated>;
	readonly updateOidcSettingsUseCase: UseCase<UpdateOidcSettingsCommand, AuthConfigUpdated>;
	readonly updateConfigTypeUseCase: UseCase<UpdateConfigTypeCommand, AuthConfigUpdated>;
	readonly updateAdditionalClientsUseCase: UseCase<UpdateAdditionalClientsCommand, AuthConfigUpdated>;
	readonly updateGrantedClientsUseCase: UseCase<UpdateGrantedClientsCommand, AuthConfigUpdated>;
	readonly deleteAuthConfigUseCase: UseCase<DeleteAuthConfigCommand, AuthConfigDeleted>;
}

/**
 * Convert ClientAuthConfig to response.
 */
function toResponse(config: ClientAuthConfig): AuthConfigResponse {
	return {
		id: config.id,
		emailDomain: config.emailDomain,
		configType: config.configType,
		primaryClientId: config.primaryClientId,
		additionalClientIds: [...config.additionalClientIds],
		grantedClientIds: [...config.grantedClientIds],
		authProvider: config.authProvider,
		oidcIssuerUrl: config.oidcIssuerUrl,
		oidcClientId: config.oidcClientId,
		oidcMultiTenant: config.oidcMultiTenant,
		oidcIssuerPattern: config.oidcIssuerPattern,
		hasClientSecret: Boolean(config.oidcClientSecretRef),
		createdAt: config.createdAt.toISOString(),
		updatedAt: config.updatedAt.toISOString(),
	};
}

/**
 * Register auth config admin API routes.
 */
export async function registerAuthConfigsRoutes(
	fastify: FastifyInstance,
	deps: AuthConfigsRoutesDeps,
): Promise<void> {
	const {
		clientAuthConfigRepository,
		createInternalAuthConfigUseCase,
		createOidcAuthConfigUseCase,
		updateOidcSettingsUseCase,
		updateConfigTypeUseCase,
		updateAdditionalClientsUseCase,
		updateGrantedClientsUseCase,
		deleteAuthConfigUseCase,
	} = deps;

	// GET /api/admin/auth-configs - List all auth configs
	fastify.get(
		'/auth-configs',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const query = request.query as { clientId?: string; configType?: string };

			let configs: ClientAuthConfig[];
			if (query.clientId) {
				configs = await clientAuthConfigRepository.findByPrimaryClientId(query.clientId);
			} else if (query.configType) {
				configs = await clientAuthConfigRepository.findByConfigType(query.configType as AuthConfigType);
			} else {
				configs = await clientAuthConfigRepository.findAll();
			}

			const response: AuthConfigListResponse = {
				configs: configs.map(toResponse),
				total: configs.length,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/auth-configs/:id - Get auth config by ID
	fastify.get(
		'/auth-configs/:id',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const config = await clientAuthConfigRepository.findById(id);

			if (!config) {
				return notFound(reply, `Auth config not found: ${id}`);
			}

			return jsonSuccess(reply, toResponse(config));
		},
	);

	// GET /api/admin/auth-configs/by-domain/:domain - Get auth config by email domain
	fastify.get(
		'/auth-configs/by-domain/:domain',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { domain } = request.params as { domain: string };
			const config = await clientAuthConfigRepository.findByEmailDomain(domain);

			if (!config) {
				return notFound(reply, `No auth config for domain: ${domain}`);
			}

			return jsonSuccess(reply, toResponse(config));
		},
	);

	// POST /api/admin/auth-configs/internal - Create INTERNAL auth config
	fastify.post(
		'/auth-configs/internal',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.CREATE),
		},
		async (request, reply) => {
			const bodyResult = safeValidate(request.body, CreateInternalAuthConfigSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as CreateInternalAuthConfigBody;
			const ctx = request.executionContext;

			const command: CreateInternalAuthConfigCommand = {
				emailDomain: body.emailDomain,
				configType: body.configType,
				primaryClientId: body.primaryClientId,
				additionalClientIds: body.additionalClientIds,
				grantedClientIds: body.grantedClientIds,
			};

			const result = await createInternalAuthConfigUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const config = await clientAuthConfigRepository.findById(result.value.getData().authConfigId);
				if (config) {
					return jsonCreated(reply, toResponse(config));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/auth-configs/oidc - Create OIDC auth config
	fastify.post(
		'/auth-configs/oidc',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.CREATE),
		},
		async (request, reply) => {
			const bodyResult = safeValidate(request.body, CreateOidcAuthConfigSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as CreateOidcAuthConfigBody;
			const ctx = request.executionContext;

			const command: CreateOidcAuthConfigCommand = {
				emailDomain: body.emailDomain,
				configType: body.configType,
				primaryClientId: body.primaryClientId,
				additionalClientIds: body.additionalClientIds,
				grantedClientIds: body.grantedClientIds,
				oidcIssuerUrl: body.oidcIssuerUrl,
				oidcClientId: body.oidcClientId,
				oidcClientSecretRef: body.oidcClientSecretRef,
				oidcMultiTenant: body.oidcMultiTenant,
				oidcIssuerPattern: body.oidcIssuerPattern,
			};

			const result = await createOidcAuthConfigUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const config = await clientAuthConfigRepository.findById(result.value.getData().authConfigId);
				if (config) {
					return jsonCreated(reply, toResponse(config));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /api/admin/auth-configs/:id/oidc - Update OIDC settings
	fastify.put(
		'/auth-configs/:id/oidc',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateOidcSettingsSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateOidcSettingsBody;
			const ctx = request.executionContext;

			const command: UpdateOidcSettingsCommand = {
				authConfigId: id,
				oidcIssuerUrl: body.oidcIssuerUrl,
				oidcClientId: body.oidcClientId,
				oidcClientSecretRef: body.oidcClientSecretRef,
				oidcMultiTenant: body.oidcMultiTenant,
				oidcIssuerPattern: body.oidcIssuerPattern,
			};

			const result = await updateOidcSettingsUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const config = await clientAuthConfigRepository.findById(id);
				if (config) {
					return jsonSuccess(reply, toResponse(config));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /api/admin/auth-configs/:id/config-type - Update config type
	fastify.put(
		'/auth-configs/:id/config-type',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateConfigTypeSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateConfigTypeBody;
			const ctx = request.executionContext;

			const command: UpdateConfigTypeCommand = {
				authConfigId: id,
				configType: body.configType,
				primaryClientId: body.primaryClientId,
			};

			const result = await updateConfigTypeUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const config = await clientAuthConfigRepository.findById(id);
				if (config) {
					return jsonSuccess(reply, toResponse(config));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /api/admin/auth-configs/:id/additional-clients - Update additional clients
	fastify.put(
		'/auth-configs/:id/additional-clients',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateAdditionalClientsSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateAdditionalClientsBody;
			const ctx = request.executionContext;

			const command: UpdateAdditionalClientsCommand = {
				authConfigId: id,
				additionalClientIds: body.clientIds,
			};

			const result = await updateAdditionalClientsUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const config = await clientAuthConfigRepository.findById(id);
				if (config) {
					return jsonSuccess(reply, toResponse(config));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /api/admin/auth-configs/:id/granted-clients - Update granted clients
	fastify.put(
		'/auth-configs/:id/granted-clients',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateGrantedClientsSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateGrantedClientsBody;
			const ctx = request.executionContext;

			const command: UpdateGrantedClientsCommand = {
				authConfigId: id,
				grantedClientIds: body.clientIds,
			};

			const result = await updateGrantedClientsUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const config = await clientAuthConfigRepository.findById(id);
				if (config) {
					return jsonSuccess(reply, toResponse(config));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/auth-configs/:id - Delete auth config
	fastify.delete(
		'/auth-configs/:id',
		{
			preHandler: requirePermission(AUTH_CONFIG_PERMISSIONS.DELETE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const ctx = request.executionContext;

			const command: DeleteAuthConfigCommand = {
				authConfigId: id,
			};

			const result = await deleteAuthConfigUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);
}
