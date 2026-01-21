/**
 * OAuth Clients Admin API
 *
 * REST endpoints for OAuth client management.
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
	CreateOAuthClientCommand,
	UpdateOAuthClientCommand,
	RegenerateOAuthClientSecretCommand,
	DeleteOAuthClientCommand,
} from '../../application/index.js';
import type {
	OAuthClientCreated,
	OAuthClientUpdated,
	OAuthClientSecretRegenerated,
	OAuthClientDeleted,
	OAuthClient,
	OAuthClientType,
	OAuthGrantType,
} from '../../domain/index.js';
import type { OAuthClientRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { OAUTH_CLIENT_PERMISSIONS } from '../../authorization/permissions/platform-auth.js';

// Request schemas using TypeBox
const OAuthClientTypeSchema = Type.Union([Type.Literal('PUBLIC'), Type.Literal('CONFIDENTIAL')]);

const OAuthGrantTypeSchema = Type.Union([
	Type.Literal('authorization_code'),
	Type.Literal('client_credentials'),
	Type.Literal('refresh_token'),
	Type.Literal('password'),
]);

const CreateOAuthClientSchema = Type.Object({
	clientId: Type.String({ minLength: 1 }),
	clientName: Type.String({ minLength: 1 }),
	clientType: OAuthClientTypeSchema,
	clientSecretRef: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	redirectUris: Type.Optional(Type.Array(Type.String())),
	allowedOrigins: Type.Optional(Type.Array(Type.String())),
	grantTypes: Type.Optional(Type.Array(OAuthGrantTypeSchema)),
	defaultScopes: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	pkceRequired: Type.Optional(Type.Boolean()),
	applicationIds: Type.Optional(Type.Array(Type.String())),
});

const UpdateOAuthClientSchema = Type.Object({
	clientName: Type.Optional(Type.String({ minLength: 1 })),
	redirectUris: Type.Optional(Type.Array(Type.String())),
	allowedOrigins: Type.Optional(Type.Array(Type.String())),
	grantTypes: Type.Optional(Type.Array(OAuthGrantTypeSchema)),
	defaultScopes: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	pkceRequired: Type.Optional(Type.Boolean()),
	applicationIds: Type.Optional(Type.Array(Type.String())),
	active: Type.Optional(Type.Boolean()),
});

const RegenerateSecretSchema = Type.Object({
	newSecretRef: Type.String({ minLength: 1 }),
});

type CreateOAuthClientBody = Static<typeof CreateOAuthClientSchema>;
type UpdateOAuthClientBody = Static<typeof UpdateOAuthClientSchema>;
type RegenerateSecretBody = Static<typeof RegenerateSecretSchema>;

// Response schemas
interface OAuthClientResponse {
	id: string;
	clientId: string;
	clientName: string;
	clientType: OAuthClientType;
	hasClientSecret: boolean;
	redirectUris: string[];
	allowedOrigins: string[];
	grantTypes: OAuthGrantType[];
	defaultScopes: string | null;
	pkceRequired: boolean;
	applicationIds: string[];
	serviceAccountPrincipalId: string | null;
	active: boolean;
	createdAt: string;
	updatedAt: string;
}

interface OAuthClientListResponse {
	clients: OAuthClientResponse[];
	total: number;
}

/**
 * Dependencies for the OAuth clients API.
 */
export interface OAuthClientsRoutesDeps {
	readonly oauthClientRepository: OAuthClientRepository;
	readonly createOAuthClientUseCase: UseCase<CreateOAuthClientCommand, OAuthClientCreated>;
	readonly updateOAuthClientUseCase: UseCase<UpdateOAuthClientCommand, OAuthClientUpdated>;
	readonly regenerateOAuthClientSecretUseCase: UseCase<RegenerateOAuthClientSecretCommand, OAuthClientSecretRegenerated>;
	readonly deleteOAuthClientUseCase: UseCase<DeleteOAuthClientCommand, OAuthClientDeleted>;
}

/**
 * Convert OAuthClient to response.
 */
function toResponse(client: OAuthClient): OAuthClientResponse {
	return {
		id: client.id,
		clientId: client.clientId,
		clientName: client.clientName,
		clientType: client.clientType,
		hasClientSecret: Boolean(client.clientSecretRef),
		redirectUris: [...client.redirectUris],
		allowedOrigins: [...client.allowedOrigins],
		grantTypes: [...client.grantTypes],
		defaultScopes: client.defaultScopes,
		pkceRequired: client.pkceRequired,
		applicationIds: [...client.applicationIds],
		serviceAccountPrincipalId: client.serviceAccountPrincipalId,
		active: client.active,
		createdAt: client.createdAt.toISOString(),
		updatedAt: client.updatedAt.toISOString(),
	};
}

/**
 * Register OAuth client admin API routes.
 */
export async function registerOAuthClientsRoutes(
	fastify: FastifyInstance,
	deps: OAuthClientsRoutesDeps,
): Promise<void> {
	const {
		oauthClientRepository,
		createOAuthClientUseCase,
		updateOAuthClientUseCase,
		regenerateOAuthClientSecretUseCase,
		deleteOAuthClientUseCase,
	} = deps;

	// GET /api/admin/oauth-clients - List all OAuth clients
	fastify.get(
		'/oauth-clients',
		{
			preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const query = request.query as { active?: string };

			let clients: OAuthClient[];
			if (query.active === 'true') {
				clients = await oauthClientRepository.findActive();
			} else {
				clients = await oauthClientRepository.findAll();
			}

			const response: OAuthClientListResponse = {
				clients: clients.map(toResponse),
				total: clients.length,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/oauth-clients/:id - Get OAuth client by ID
	fastify.get(
		'/oauth-clients/:id',
		{
			preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const client = await oauthClientRepository.findById(id);

			if (!client) {
				return notFound(reply, `OAuth client not found: ${id}`);
			}

			return jsonSuccess(reply, toResponse(client));
		},
	);

	// GET /api/admin/oauth-clients/by-client-id/:clientId - Get OAuth client by clientId
	fastify.get(
		'/oauth-clients/by-client-id/:clientId',
		{
			preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { clientId } = request.params as { clientId: string };
			const client = await oauthClientRepository.findByClientId(clientId);

			if (!client) {
				return notFound(reply, `OAuth client not found: ${clientId}`);
			}

			return jsonSuccess(reply, toResponse(client));
		},
	);

	// POST /api/admin/oauth-clients - Create OAuth client
	fastify.post(
		'/oauth-clients',
		{
			preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.CREATE),
		},
		async (request, reply) => {
			const bodyResult = safeValidate(request.body, CreateOAuthClientSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as CreateOAuthClientBody;
			const ctx = request.executionContext;

			const command: CreateOAuthClientCommand = {
				clientId: body.clientId,
				clientName: body.clientName,
				clientType: body.clientType,
				clientSecretRef: body.clientSecretRef,
				redirectUris: body.redirectUris,
				allowedOrigins: body.allowedOrigins,
				grantTypes: body.grantTypes,
				defaultScopes: body.defaultScopes,
				pkceRequired: body.pkceRequired,
				applicationIds: body.applicationIds,
			};

			const result = await createOAuthClientUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const client = await oauthClientRepository.findById(result.value.getData().oauthClientId);
				if (client) {
					return jsonCreated(reply, toResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /api/admin/oauth-clients/:id - Update OAuth client
	fastify.put(
		'/oauth-clients/:id',
		{
			preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateOAuthClientSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateOAuthClientBody;
			const ctx = request.executionContext;

			const command: UpdateOAuthClientCommand = {
				oauthClientId: id,
				clientName: body.clientName,
				redirectUris: body.redirectUris,
				allowedOrigins: body.allowedOrigins,
				grantTypes: body.grantTypes,
				defaultScopes: body.defaultScopes,
				pkceRequired: body.pkceRequired,
				applicationIds: body.applicationIds,
				active: body.active,
			};

			const result = await updateOAuthClientUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const client = await oauthClientRepository.findById(id);
				if (client) {
					return jsonSuccess(reply, toResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// POST /api/admin/oauth-clients/:id/regenerate-secret - Regenerate client secret
	fastify.post(
		'/oauth-clients/:id/regenerate-secret',
		{
			preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.REGENERATE_SECRET),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, RegenerateSecretSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as RegenerateSecretBody;
			const ctx = request.executionContext;

			const command: RegenerateOAuthClientSecretCommand = {
				oauthClientId: id,
				newSecretRef: body.newSecretRef,
			};

			const result = await regenerateOAuthClientSecretUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const client = await oauthClientRepository.findById(id);
				if (client) {
					return jsonSuccess(reply, toResponse(client));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/oauth-clients/:id - Delete OAuth client
	fastify.delete(
		'/oauth-clients/:id',
		{
			preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.DELETE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const ctx = request.executionContext;

			const command: DeleteOAuthClientCommand = {
				oauthClientId: id,
			};

			const result = await deleteOAuthClientUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);
}
