/**
 * Email Domain Mappings Admin API
 *
 * REST endpoints for email domain mapping management.
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
	CreateEmailDomainMappingCommand,
	UpdateEmailDomainMappingCommand,
	DeleteEmailDomainMappingCommand,
} from "../../application/index.js";
import type {
	EmailDomainMappingCreated,
	EmailDomainMappingUpdated,
	EmailDomainMappingDeleted,
	EmailDomainMapping,
} from "../../domain/index.js";
import type {
	EmailDomainMappingRepository,
	ClientRepository,
} from "../../infrastructure/persistence/index.js";
import type { IdentityProviderRepository } from "../../infrastructure/persistence/repositories/identity-provider-repository.js";
import { requirePermission } from "../../authorization/index.js";
import { EMAIL_DOMAIN_MAPPING_PERMISSIONS } from "../../authorization/permissions/platform-admin.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const ScopeTypeSchema = Type.Union([
	Type.Literal("ANCHOR"),
	Type.Literal("PARTNER"),
	Type.Literal("CLIENT"),
]);

const CreateEmailDomainMappingSchema = Type.Object({
	emailDomain: Type.String({ minLength: 1, maxLength: 255 }),
	identityProviderId: Type.String({ minLength: 1 }),
	scopeType: ScopeTypeSchema,
	primaryClientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	additionalClientIds: Type.Optional(Type.Array(Type.String())),
	grantedClientIds: Type.Optional(Type.Array(Type.String())),
	requiredOidcTenantId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	allowedRoleIds: Type.Optional(Type.Array(Type.String())),
	syncRolesFromIdp: Type.Optional(Type.Boolean()),
});

const UpdateEmailDomainMappingSchema = Type.Object({
	identityProviderId: Type.Optional(Type.String({ minLength: 1 })),
	scopeType: Type.Optional(ScopeTypeSchema),
	primaryClientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	additionalClientIds: Type.Optional(Type.Array(Type.String())),
	grantedClientIds: Type.Optional(Type.Array(Type.String())),
	requiredOidcTenantId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
	allowedRoleIds: Type.Optional(Type.Array(Type.String())),
	syncRolesFromIdp: Type.Optional(Type.Boolean()),
});

const IdParam = Type.Object({ id: Type.String() });
const DomainParam = Type.Object({ domain: Type.String() });

// ─── Response Schemas ───────────────────────────────────────────────────────

const EmailDomainMappingResponseSchema = Type.Object({
	id: Type.String(),
	emailDomain: Type.String(),
	identityProviderId: Type.String(),
	identityProviderName: Type.Union([Type.String(), Type.Null()]),
	identityProviderType: Type.Union([Type.String(), Type.Null()]),
	scopeType: Type.String(),
	primaryClientId: Type.Union([Type.String(), Type.Null()]),
	primaryClientName: Type.Union([Type.String(), Type.Null()]),
	additionalClientIds: Type.Array(Type.String()),
	grantedClientIds: Type.Array(Type.String()),
	requiredOidcTenantId: Type.Union([Type.String(), Type.Null()]),
	allowedRoleIds: Type.Array(Type.String()),
	syncRolesFromIdp: Type.Boolean(),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const EmailDomainMappingListResponseSchema = Type.Object({
	mappings: Type.Array(EmailDomainMappingResponseSchema),
	total: Type.Integer(),
});

type EmailDomainMappingResponse = Static<
	typeof EmailDomainMappingResponseSchema
>;

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface EmailDomainMappingsRoutesDeps {
	readonly emailDomainMappingRepository: EmailDomainMappingRepository;
	readonly identityProviderRepository: IdentityProviderRepository;
	readonly clientRepository: ClientRepository;
	readonly createEmailDomainMappingUseCase: UseCase<
		CreateEmailDomainMappingCommand,
		EmailDomainMappingCreated
	>;
	readonly updateEmailDomainMappingUseCase: UseCase<
		UpdateEmailDomainMappingCommand,
		EmailDomainMappingUpdated
	>;
	readonly deleteEmailDomainMappingUseCase: UseCase<
		DeleteEmailDomainMappingCommand,
		EmailDomainMappingDeleted
	>;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerEmailDomainMappingsRoutes(
	fastify: FastifyInstance,
	deps: EmailDomainMappingsRoutesDeps,
): Promise<void> {
	const {
		emailDomainMappingRepository,
		identityProviderRepository,
		clientRepository,
		createEmailDomainMappingUseCase,
		updateEmailDomainMappingUseCase,
		deleteEmailDomainMappingUseCase,
	} = deps;

	// GET /email-domain-mappings - List all
	fastify.get(
		"/email-domain-mappings",
		{
			preHandler: requirePermission(EMAIL_DOMAIN_MAPPING_PERMISSIONS.READ),
			schema: {
				response: {
					200: EmailDomainMappingListResponseSchema,
				},
			},
		},
		async (_request, reply) => {
			const mappings = await emailDomainMappingRepository.findAll();

			// Batch-load identity providers for enrichment
			const idpIds = [...new Set(mappings.map((m) => m.identityProviderId))];
			const idpMap = new Map<string, { name: string; type: string }>();
			for (const idpId of idpIds) {
				const idp = await identityProviderRepository.findById(idpId);
				if (idp) {
					idpMap.set(idpId, { name: idp.name, type: idp.type });
				}
			}

			// Batch-load primary client names
			const clientIds = [
				...new Set(mappings.map((m) => m.primaryClientId).filter(Boolean)),
			] as string[];
			const clientNameMap = new Map<string, string>();
			for (const clientId of clientIds) {
				const client = await clientRepository.findById(clientId);
				if (client) {
					clientNameMap.set(clientId, client.name);
				}
			}

			return jsonSuccess(reply, {
				mappings: mappings.map((m) =>
					toEmailDomainMappingResponse(
						m,
						idpMap.get(m.identityProviderId),
						m.primaryClientId ? clientNameMap.get(m.primaryClientId) : null,
					),
				),
				total: mappings.length,
			});
		},
	);

	// GET /email-domain-mappings/:id - Get by ID
	fastify.get(
		"/email-domain-mappings/:id",
		{
			preHandler: requirePermission(EMAIL_DOMAIN_MAPPING_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: EmailDomainMappingResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const mapping = await emailDomainMappingRepository.findById(id);
			if (!mapping) {
				return notFound(reply, "Email domain mapping not found");
			}
			const idp = await identityProviderRepository.findById(
				mapping.identityProviderId,
			);
			const primaryClient = mapping.primaryClientId
				? await clientRepository.findById(mapping.primaryClientId)
				: null;
			return jsonSuccess(
				reply,
				toEmailDomainMappingResponse(
					mapping,
					idp ? { name: idp.name, type: idp.type } : undefined,
					primaryClient?.name,
				),
			);
		},
	);

	// GET /email-domain-mappings/lookup/:domain - Lookup by domain
	fastify.get(
		"/email-domain-mappings/lookup/:domain",
		{
			preHandler: requirePermission(EMAIL_DOMAIN_MAPPING_PERMISSIONS.READ),
			schema: {
				params: DomainParam,
				response: {
					200: EmailDomainMappingResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { domain } = request.params as Static<typeof DomainParam>;
			const mapping =
				await emailDomainMappingRepository.findByEmailDomain(domain);
			if (!mapping) {
				return notFound(reply, "No mapping found for this email domain");
			}
			const idp = await identityProviderRepository.findById(
				mapping.identityProviderId,
			);
			const primaryClient = mapping.primaryClientId
				? await clientRepository.findById(mapping.primaryClientId)
				: null;
			return jsonSuccess(
				reply,
				toEmailDomainMappingResponse(
					mapping,
					idp ? { name: idp.name, type: idp.type } : undefined,
					primaryClient?.name,
				),
			);
		},
	);

	// POST /email-domain-mappings - Create
	fastify.post(
		"/email-domain-mappings",
		{
			preHandler: requirePermission(EMAIL_DOMAIN_MAPPING_PERMISSIONS.CREATE),
			schema: {
				body: CreateEmailDomainMappingSchema,
				response: {
					201: EmailDomainMappingResponseSchema,
					400: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const body = request.body as Static<
				typeof CreateEmailDomainMappingSchema
			>;
			const ctx = request.executionContext;

			const command: CreateEmailDomainMappingCommand = {
				emailDomain: body.emailDomain,
				identityProviderId: body.identityProviderId,
				scopeType: body.scopeType,
				primaryClientId: body.primaryClientId,
				additionalClientIds: body.additionalClientIds,
				grantedClientIds: body.grantedClientIds,
				requiredOidcTenantId: body.requiredOidcTenantId,
				allowedRoleIds: body.allowedRoleIds,
				syncRolesFromIdp: body.syncRolesFromIdp,
			};

			const result = await createEmailDomainMappingUseCase.execute(
				command,
				ctx,
			);

			if (Result.isSuccess(result)) {
				const mapping = await emailDomainMappingRepository.findById(
					result.value.getData().emailDomainMappingId,
				);
				if (mapping) {
					const idp = await identityProviderRepository.findById(
						mapping.identityProviderId,
					);
					const primaryClient = mapping.primaryClientId
						? await clientRepository.findById(mapping.primaryClientId)
						: null;
					return jsonCreated(
						reply,
						toEmailDomainMappingResponse(
							mapping,
							idp ? { name: idp.name, type: idp.type } : undefined,
							primaryClient?.name,
						),
					);
				}
			}

			return sendResult(reply, result);
		},
	);

	// PUT /email-domain-mappings/:id - Update
	fastify.put(
		"/email-domain-mappings/:id",
		{
			preHandler: requirePermission(EMAIL_DOMAIN_MAPPING_PERMISSIONS.UPDATE),
			schema: {
				params: IdParam,
				body: UpdateEmailDomainMappingSchema,
				response: {
					200: EmailDomainMappingResponseSchema,
					400: ErrorResponseSchema,
					404: ErrorResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const body = request.body as Static<
				typeof UpdateEmailDomainMappingSchema
			>;
			const ctx = request.executionContext;

			const command: UpdateEmailDomainMappingCommand = {
				emailDomainMappingId: id,
				...(body.identityProviderId !== undefined
					? { identityProviderId: body.identityProviderId }
					: {}),
				...(body.scopeType !== undefined ? { scopeType: body.scopeType } : {}),
				...(body.primaryClientId !== undefined
					? { primaryClientId: body.primaryClientId }
					: {}),
				...(body.additionalClientIds !== undefined
					? { additionalClientIds: body.additionalClientIds }
					: {}),
				...(body.grantedClientIds !== undefined
					? { grantedClientIds: body.grantedClientIds }
					: {}),
				...(body.requiredOidcTenantId !== undefined
					? { requiredOidcTenantId: body.requiredOidcTenantId }
					: {}),
				...(body.allowedRoleIds !== undefined
					? { allowedRoleIds: body.allowedRoleIds }
					: {}),
				...(body.syncRolesFromIdp !== undefined
					? { syncRolesFromIdp: body.syncRolesFromIdp }
					: {}),
			};

			const result = await updateEmailDomainMappingUseCase.execute(
				command,
				ctx,
			);

			if (Result.isSuccess(result)) {
				const mapping = await emailDomainMappingRepository.findById(id);
				if (mapping) {
					const idp = await identityProviderRepository.findById(
						mapping.identityProviderId,
					);
					const primaryClient = mapping.primaryClientId
						? await clientRepository.findById(mapping.primaryClientId)
						: null;
					return jsonSuccess(
						reply,
						toEmailDomainMappingResponse(
							mapping,
							idp ? { name: idp.name, type: idp.type } : undefined,
							primaryClient?.name,
						),
					);
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /email-domain-mappings/:id - Delete
	fastify.delete(
		"/email-domain-mappings/:id",
		{
			preHandler: requirePermission(EMAIL_DOMAIN_MAPPING_PERMISSIONS.DELETE),
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

			const command: DeleteEmailDomainMappingCommand = {
				emailDomainMappingId: id,
			};
			const result = await deleteEmailDomainMappingUseCase.execute(
				command,
				ctx,
			);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toEmailDomainMappingResponse(
	mapping: EmailDomainMapping,
	idp?: { name: string; type: string },
	clientName?: string | null,
): EmailDomainMappingResponse {
	return {
		id: mapping.id,
		emailDomain: mapping.emailDomain,
		identityProviderId: mapping.identityProviderId,
		identityProviderName: idp?.name ?? null,
		identityProviderType: idp?.type ?? null,
		scopeType: mapping.scopeType,
		primaryClientId: mapping.primaryClientId,
		primaryClientName: clientName ?? null,
		additionalClientIds: [...mapping.additionalClientIds],
		grantedClientIds: [...mapping.grantedClientIds],
		requiredOidcTenantId: mapping.requiredOidcTenantId,
		allowedRoleIds: [...mapping.allowedRoleIds],
		syncRolesFromIdp: mapping.syncRolesFromIdp,
		createdAt: mapping.createdAt.toISOString(),
		updatedAt: mapping.updatedAt.toISOString(),
	};
}
