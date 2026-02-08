/**
 * Identity Providers Admin API
 *
 * REST endpoints for identity provider management.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import { sendResult, jsonCreated, jsonSuccess, noContent, notFound, ErrorResponseSchema } from '@flowcatalyst/http';
import { Result } from '@flowcatalyst/application';
import type { UseCase } from '@flowcatalyst/application';

import type {
	CreateIdentityProviderCommand,
	UpdateIdentityProviderCommand,
	DeleteIdentityProviderCommand,
} from '../../application/index.js';
import type {
	IdentityProviderCreated,
	IdentityProviderUpdated,
	IdentityProviderDeleted,
	IdentityProvider,
} from '../../domain/index.js';
import type { IdentityProviderRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { IDENTITY_PROVIDER_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateIdentityProviderSchema = Type.Object({
	code: Type.String({ minLength: 2, maxLength: 50, pattern: '^[a-z][a-z0-9-]*$' }),
	name: Type.String({ minLength: 1, maxLength: 200 }),
	type: Type.Union([Type.Literal('INTERNAL'), Type.Literal('OIDC')]),
	oidcIssuerUrl: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	oidcClientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	oidcClientSecretRef: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	oidcMultiTenant: Type.Optional(Type.Boolean()),
	oidcIssuerPattern: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	allowedEmailDomains: Type.Optional(Type.Array(Type.String())),
});

const UpdateIdentityProviderSchema = Type.Object({
	name: Type.Optional(Type.String({ minLength: 1, maxLength: 200 })),
	type: Type.Optional(Type.Union([Type.Literal('INTERNAL'), Type.Literal('OIDC')])),
	oidcIssuerUrl: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	oidcClientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	oidcClientSecretRef: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	oidcMultiTenant: Type.Optional(Type.Boolean()),
	oidcIssuerPattern: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	allowedEmailDomains: Type.Optional(Type.Array(Type.String())),
});

const IdParam = Type.Object({ id: Type.String() });

// ─── Response Schemas ───────────────────────────────────────────────────────

const IdentityProviderResponseSchema = Type.Object({
	id: Type.String(),
	code: Type.String(),
	name: Type.String(),
	type: Type.String(),
	oidcIssuerUrl: Type.Union([Type.String(), Type.Null()]),
	oidcClientId: Type.Union([Type.String(), Type.Null()]),
	oidcClientSecretRef: Type.Union([Type.String(), Type.Null()]),
	oidcMultiTenant: Type.Boolean(),
	oidcIssuerPattern: Type.Union([Type.String(), Type.Null()]),
	allowedEmailDomains: Type.Array(Type.String()),
	createdAt: Type.String({ format: 'date-time' }),
	updatedAt: Type.String({ format: 'date-time' }),
});

const IdentityProviderListResponseSchema = Type.Object({
	identityProviders: Type.Array(IdentityProviderResponseSchema),
	total: Type.Integer(),
});

type IdentityProviderResponse = Static<typeof IdentityProviderResponseSchema>;

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface IdentityProvidersRoutesDeps {
	readonly identityProviderRepository: IdentityProviderRepository;
	readonly createIdentityProviderUseCase: UseCase<CreateIdentityProviderCommand, IdentityProviderCreated>;
	readonly updateIdentityProviderUseCase: UseCase<UpdateIdentityProviderCommand, IdentityProviderUpdated>;
	readonly deleteIdentityProviderUseCase: UseCase<DeleteIdentityProviderCommand, IdentityProviderDeleted>;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerIdentityProvidersRoutes(
	fastify: FastifyInstance,
	deps: IdentityProvidersRoutesDeps,
): Promise<void> {
	const {
		identityProviderRepository,
		createIdentityProviderUseCase,
		updateIdentityProviderUseCase,
		deleteIdentityProviderUseCase,
	} = deps;

	// GET /identity-providers - List all
	fastify.get(
		'/identity-providers',
		{
			preHandler: requirePermission(IDENTITY_PROVIDER_PERMISSIONS.READ),
			schema: {
				response: {
					200: IdentityProviderListResponseSchema,
				},
			},
		},
		async (_request, reply) => {
			const providers = await identityProviderRepository.findAll();
			return jsonSuccess(reply, {
				identityProviders: providers.map(toIdentityProviderResponse),
				total: providers.length,
			});
		},
	);

	// GET /identity-providers/:id - Get by ID
	fastify.get(
		'/identity-providers/:id',
		{
			preHandler: requirePermission(IDENTITY_PROVIDER_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: IdentityProviderResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const provider = await identityProviderRepository.findById(id);
			if (!provider) {
				return notFound(reply, 'Identity provider not found');
			}
			return jsonSuccess(reply, toIdentityProviderResponse(provider));
		},
	);

	// POST /identity-providers - Create
	fastify.post(
		'/identity-providers',
		{
			preHandler: requirePermission(IDENTITY_PROVIDER_PERMISSIONS.CREATE),
			schema: {
				body: CreateIdentityProviderSchema,
				response: {
					201: IdentityProviderResponseSchema,
					400: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as Static<typeof CreateIdentityProviderSchema>;
			const ctx = request.executionContext;

			const command: CreateIdentityProviderCommand = {
				code: body.code,
				name: body.name,
				type: body.type,
				oidcIssuerUrl: body.oidcIssuerUrl,
				oidcClientId: body.oidcClientId,
				oidcClientSecretRef: body.oidcClientSecretRef,
				oidcMultiTenant: body.oidcMultiTenant,
				oidcIssuerPattern: body.oidcIssuerPattern,
				allowedEmailDomains: body.allowedEmailDomains,
			};

			const result = await createIdentityProviderUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const provider = await identityProviderRepository.findById(result.value.getData().identityProviderId);
				if (provider) {
					return jsonCreated(reply, toIdentityProviderResponse(provider));
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /identity-providers/:id - Update
	fastify.put(
		'/identity-providers/:id',
		{
			preHandler: requirePermission(IDENTITY_PROVIDER_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				body: UpdateIdentityProviderSchema,
				response: {
					200: IdentityProviderResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const body = request.body as Static<typeof UpdateIdentityProviderSchema>;
			const ctx = request.executionContext;

			const command: UpdateIdentityProviderCommand = {
				identityProviderId: id,
				...(body.name !== undefined ? { name: body.name } : {}),
				...(body.type !== undefined ? { type: body.type } : {}),
				...(body.oidcIssuerUrl !== undefined ? { oidcIssuerUrl: body.oidcIssuerUrl } : {}),
				...(body.oidcClientId !== undefined ? { oidcClientId: body.oidcClientId } : {}),
				...(body.oidcClientSecretRef !== undefined ? { oidcClientSecretRef: body.oidcClientSecretRef } : {}),
				...(body.oidcMultiTenant !== undefined ? { oidcMultiTenant: body.oidcMultiTenant } : {}),
				...(body.oidcIssuerPattern !== undefined ? { oidcIssuerPattern: body.oidcIssuerPattern } : {}),
				...(body.allowedEmailDomains !== undefined ? { allowedEmailDomains: body.allowedEmailDomains } : {}),
			};

			const result = await updateIdentityProviderUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const provider = await identityProviderRepository.findById(id);
				if (provider) {
					return jsonSuccess(reply, toIdentityProviderResponse(provider));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /identity-providers/:id - Delete
	fastify.delete(
		'/identity-providers/:id',
		{
			preHandler: requirePermission(IDENTITY_PROVIDER_PERMISSIONS.DELETE),
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

			const command: DeleteIdentityProviderCommand = { identityProviderId: id };
			const result = await deleteIdentityProviderUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toIdentityProviderResponse(idp: IdentityProvider): IdentityProviderResponse {
	return {
		id: idp.id,
		code: idp.code,
		name: idp.name,
		type: idp.type,
		oidcIssuerUrl: idp.oidcIssuerUrl,
		oidcClientId: idp.oidcClientId,
		oidcClientSecretRef: idp.oidcClientSecretRef,
		oidcMultiTenant: idp.oidcMultiTenant,
		oidcIssuerPattern: idp.oidcIssuerPattern,
		allowedEmailDomains: [...idp.allowedEmailDomains],
		createdAt: idp.createdAt.toISOString(),
		updatedAt: idp.updatedAt.toISOString(),
	};
}
