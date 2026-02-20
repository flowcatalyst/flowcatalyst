/**
 * Config Access Admin API
 *
 * REST endpoints for managing which roles can read/write platform configuration.
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import {
	jsonCreated,
	jsonSuccess,
	noContent,
	notFound,
	jsonError,
	ErrorResponseSchema,
} from "@flowcatalyst/http";

import type { PlatformConfigAccessRepository } from "../../infrastructure/persistence/index.js";
import {
	createPlatformConfigAccess,
	type PlatformConfigAccess,
} from "../../domain/index.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const AppCodeParam = Type.Object({ appCode: Type.String() });
const RoleParam = Type.Object({
	appCode: Type.String(),
	roleCode: Type.String(),
});

const GrantAccessSchema = Type.Object({
	roleCode: Type.String({ minLength: 1 }),
	canRead: Type.Optional(Type.Boolean()),
	canWrite: Type.Optional(Type.Boolean()),
});

const UpdateAccessSchema = Type.Object({
	canRead: Type.Optional(Type.Boolean()),
	canWrite: Type.Optional(Type.Boolean()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const AccessGrantResponseSchema = Type.Object({
	id: Type.String(),
	applicationCode: Type.String(),
	roleCode: Type.String(),
	canRead: Type.Boolean(),
	canWrite: Type.Boolean(),
	createdAt: Type.String({ format: "date-time" }),
});

const AccessGrantListResponseSchema = Type.Object({
	items: Type.Array(AccessGrantResponseSchema),
});

type AccessGrantResponse = Static<typeof AccessGrantResponseSchema>;

/**
 * Dependencies for the config access admin API.
 */
export interface ConfigAccessRoutesDeps {
	readonly platformConfigAccessRepository: PlatformConfigAccessRepository;
}

/**
 * Register config access admin API routes.
 */
export async function registerConfigAccessRoutes(
	fastify: FastifyInstance,
	deps: ConfigAccessRoutesDeps,
): Promise<void> {
	const { platformConfigAccessRepository } = deps;

	// GET /api/admin/config-access/:appCode - List access grants
	fastify.get(
		"/config-access/:appCode",
		{
			schema: {
				params: AppCodeParam,
				response: {
					200: AccessGrantListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode } = request.params as Static<typeof AppCodeParam>;
			const grants =
				await platformConfigAccessRepository.findByApplication(appCode);

			return jsonSuccess(reply, {
				items: grants.map(toAccessGrantResponse),
			});
		},
	);

	// POST /api/admin/config-access/:appCode - Grant config access
	fastify.post(
		"/config-access/:appCode",
		{
			schema: {
				params: AppCodeParam,
				body: GrantAccessSchema,
				response: {
					201: AccessGrantResponseSchema,
					409: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode } = request.params as Static<typeof AppCodeParam>;
			const body = request.body as Static<typeof GrantAccessSchema>;

			// Check if grant already exists
			const existing =
				await platformConfigAccessRepository.findByApplicationAndRole(
					appCode,
					body.roleCode,
				);
			if (existing) {
				return jsonError(
					reply,
					409,
					"GRANT_EXISTS",
					`Access grant already exists for role: ${body.roleCode}`,
				);
			}

			const entity = createPlatformConfigAccess({
				applicationCode: appCode,
				roleCode: body.roleCode,
				canRead: body.canRead ?? true,
				canWrite: body.canWrite ?? false,
			});

			const grant = await platformConfigAccessRepository.insert(entity);
			return jsonCreated(reply, toAccessGrantResponse(grant));
		},
	);

	// PUT /api/admin/config-access/:appCode/:roleCode - Update access grant
	fastify.put(
		"/config-access/:appCode/:roleCode",
		{
			schema: {
				params: RoleParam,
				body: UpdateAccessSchema,
				response: {
					200: AccessGrantResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode, roleCode } = request.params as Static<typeof RoleParam>;
			const body = request.body as Static<typeof UpdateAccessSchema>;

			const existing =
				await platformConfigAccessRepository.findByApplicationAndRole(
					appCode,
					roleCode,
				);
			if (!existing) {
				return notFound(reply, `Access grant not found for role: ${roleCode}`);
			}

			const updated = await platformConfigAccessRepository.update({
				...existing,
				canRead: body.canRead ?? existing.canRead,
				canWrite: body.canWrite ?? existing.canWrite,
			});

			return jsonSuccess(reply, toAccessGrantResponse(updated));
		},
	);

	// DELETE /api/admin/config-access/:appCode/:roleCode - Revoke access
	fastify.delete(
		"/config-access/:appCode/:roleCode",
		{
			schema: {
				params: RoleParam,
				response: {
					204: Type.Null(),
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode, roleCode } = request.params as Static<typeof RoleParam>;

			const deleted =
				await platformConfigAccessRepository.deleteByApplicationAndRole(
					appCode,
					roleCode,
				);
			if (!deleted) {
				return notFound(reply, `Access grant not found for role: ${roleCode}`);
			}

			return noContent(reply);
		},
	);
}

function toAccessGrantResponse(
	grant: PlatformConfigAccess,
): AccessGrantResponse {
	return {
		id: grant.id,
		applicationCode: grant.applicationCode,
		roleCode: grant.roleCode,
		canRead: grant.canRead,
		canWrite: grant.canWrite,
		createdAt: grant.createdAt.toISOString(),
	};
}
