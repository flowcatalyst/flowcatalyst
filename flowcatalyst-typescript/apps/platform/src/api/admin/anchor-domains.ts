/**
 * Anchor Domains Admin API
 *
 * REST endpoints for anchor domain management.
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
	CreateAnchorDomainCommand,
	UpdateAnchorDomainCommand,
	DeleteAnchorDomainCommand,
} from '../../application/index.js';
import type { AnchorDomainCreated, AnchorDomainUpdated, AnchorDomainDeleted } from '../../domain/index.js';
import type { AnchorDomainRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { ANCHOR_DOMAIN_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// Request schemas using TypeBox
const CreateAnchorDomainSchema = Type.Object({
	domain: Type.String({ minLength: 1, maxLength: 255 }),
});

const UpdateAnchorDomainSchema = Type.Object({
	domain: Type.String({ minLength: 1, maxLength: 255 }),
});

type CreateAnchorDomainBody = Static<typeof CreateAnchorDomainSchema>;
type UpdateAnchorDomainBody = Static<typeof UpdateAnchorDomainSchema>;

// Response schemas for anchor domain
interface AnchorDomainResponse {
	id: string;
	domain: string;
	createdAt: string;
}

interface AnchorDomainsListResponse {
	anchorDomains: AnchorDomainResponse[];
	total: number;
}

/**
 * Dependencies for the anchor domains API.
 */
export interface AnchorDomainsRoutesDeps {
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly createAnchorDomainUseCase: UseCase<CreateAnchorDomainCommand, AnchorDomainCreated>;
	readonly updateAnchorDomainUseCase: UseCase<UpdateAnchorDomainCommand, AnchorDomainUpdated>;
	readonly deleteAnchorDomainUseCase: UseCase<DeleteAnchorDomainCommand, AnchorDomainDeleted>;
}

/**
 * Register anchor domain admin API routes.
 */
export async function registerAnchorDomainsRoutes(
	fastify: FastifyInstance,
	deps: AnchorDomainsRoutesDeps,
): Promise<void> {
	const {
		anchorDomainRepository,
		createAnchorDomainUseCase,
		updateAnchorDomainUseCase,
		deleteAnchorDomainUseCase,
	} = deps;

	// POST /api/admin/anchor-domains - Create anchor domain
	fastify.post(
		'/anchor-domains',
		{
			preHandler: requirePermission(ANCHOR_DOMAIN_PERMISSIONS.CREATE),
		},
		async (request, reply) => {
			const bodyResult = safeValidate(request.body, CreateAnchorDomainSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as CreateAnchorDomainBody;
			const ctx = request.executionContext;

			const command: CreateAnchorDomainCommand = {
				domain: body.domain,
			};

			const result = await createAnchorDomainUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const anchorDomain = await anchorDomainRepository.findById(result.value.getData().anchorDomainId);
				if (anchorDomain) {
					return jsonCreated(reply, toAnchorDomainResponse(anchorDomain));
				}
			}

			return sendResult(reply, result);
		},
	);

	// GET /api/admin/anchor-domains - List anchor domains
	fastify.get(
		'/anchor-domains',
		{
			preHandler: requirePermission(ANCHOR_DOMAIN_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const anchorDomains = await anchorDomainRepository.findAll();
			const total = await anchorDomainRepository.count();

			const response: AnchorDomainsListResponse = {
				anchorDomains: anchorDomains.map(toAnchorDomainResponse),
				total,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/anchor-domains/:id - Get anchor domain by ID
	fastify.get(
		'/anchor-domains/:id',
		{
			preHandler: requirePermission(ANCHOR_DOMAIN_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const anchorDomain = await anchorDomainRepository.findById(id);

			if (!anchorDomain) {
				return notFound(reply, `Anchor domain not found: ${id}`);
			}

			return jsonSuccess(reply, toAnchorDomainResponse(anchorDomain));
		},
	);

	// PUT /api/admin/anchor-domains/:id - Update anchor domain
	fastify.put(
		'/anchor-domains/:id',
		{
			preHandler: requirePermission(ANCHOR_DOMAIN_PERMISSIONS.UPDATE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const bodyResult = safeValidate(request.body, UpdateAnchorDomainSchema);
			if (!bodyResult.success) {
				return badRequest(reply, bodyResult.error);
			}

			const body = bodyResult.data as UpdateAnchorDomainBody;
			const ctx = request.executionContext;

			const command: UpdateAnchorDomainCommand = {
				anchorDomainId: id,
				domain: body.domain,
			};

			const result = await updateAnchorDomainUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				const anchorDomain = await anchorDomainRepository.findById(id);
				if (anchorDomain) {
					return jsonSuccess(reply, toAnchorDomainResponse(anchorDomain));
				}
			}

			return sendResult(reply, result);
		},
	);

	// DELETE /api/admin/anchor-domains/:id - Delete anchor domain
	fastify.delete(
		'/anchor-domains/:id',
		{
			preHandler: requirePermission(ANCHOR_DOMAIN_PERMISSIONS.DELETE),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
			const ctx = request.executionContext;

			const command: DeleteAnchorDomainCommand = {
				anchorDomainId: id,
			};

			const result = await deleteAnchorDomainUseCase.execute(command, ctx);

			if (Result.isSuccess(result)) {
				return noContent(reply);
			}

			return sendResult(reply, result);
		},
	);
}

/**
 * Convert an AnchorDomain entity to an AnchorDomainResponse.
 */
function toAnchorDomainResponse(anchorDomain: {
	id: string;
	domain: string;
	createdAt: Date;
}): AnchorDomainResponse {
	return {
		id: anchorDomain.id,
		domain: anchorDomain.domain,
		createdAt: anchorDomain.createdAt.toISOString(),
	};
}
