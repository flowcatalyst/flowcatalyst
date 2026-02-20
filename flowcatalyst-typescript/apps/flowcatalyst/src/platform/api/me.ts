/**
 * User Resource (/api/me)
 *
 * User-facing API for accessing resources the authenticated user has access to.
 * These endpoints do NOT require admin permissions - they return only resources
 * the user can access based on their scope (ANCHOR/PARTNER/CLIENT).
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import {
	jsonSuccess,
	notFound,
	forbidden,
	ErrorResponseSchema,
} from "@flowcatalyst/http";
import { requireAuthentication } from "../authorization/index.js";
import {
	canAccessClient,
	getClientQueryScopeForPrincipal,
} from "../authorization/index.js";
import type { ClientRepository } from "../infrastructure/persistence/index.js";
import type {
	ApplicationRepository,
	ApplicationClientConfigRepository,
} from "../infrastructure/persistence/index.js";

// ─── Response Schemas ───────────────────────────────────────────────────────

const MyClientSchema = Type.Object({
	id: Type.String(),
	name: Type.String(),
	identifier: Type.String(),
	status: Type.Union([Type.String(), Type.Null()]),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const MyClientsResponseSchema = Type.Object({
	clients: Type.Array(MyClientSchema),
	total: Type.Integer(),
});

const MyApplicationSchema = Type.Object({
	id: Type.String(),
	code: Type.String(),
	name: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	iconUrl: Type.Union([Type.String(), Type.Null()]),
	baseUrl: Type.Union([Type.String(), Type.Null()]),
	website: Type.Union([Type.String(), Type.Null()]),
	logoMimeType: Type.Union([Type.String(), Type.Null()]),
});

const MyApplicationsResponseSchema = Type.Object({
	applications: Type.Array(MyApplicationSchema),
	total: Type.Integer(),
	clientId: Type.String(),
});

const ClientIdParam = Type.Object({ clientId: Type.String() });

type MyClientResponse = Static<typeof MyClientSchema>;
type MyApplicationResponse = Static<typeof MyApplicationSchema>;

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface MeRoutesDeps {
	readonly clientRepository: ClientRepository;
	readonly applicationRepository: ApplicationRepository;
	readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerMeRoutes(
	fastify: FastifyInstance,
	deps: MeRoutesDeps,
): Promise<void> {
	const {
		clientRepository,
		applicationRepository,
		applicationClientConfigRepository,
	} = deps;

	// GET /api/me/clients - List accessible clients
	fastify.get(
		"/clients",
		{
			preHandler: requireAuthentication(),
			schema: {
				response: {
					200: MyClientsResponseSchema,
					401: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const principal = request.audit.principal!;
			const scope = getClientQueryScopeForPrincipal(principal);

			let clients: {
				id: string;
				name: string;
				identifier: string;
				status: string;
				createdAt: Date;
				updatedAt: Date;
			}[];

			switch (scope.type) {
				case "unrestricted":
					clients = await clientRepository.findAll();
					break;
				case "restricted":
					if (scope.filter.clientIds.length === 0) {
						clients = [];
					} else {
						// Fetch each accessible client
						const results = await Promise.all(
							scope.filter.clientIds.map((id) => clientRepository.findById(id)),
						);
						clients = results.filter(
							(c): c is NonNullable<typeof c> => c != null,
						);
					}
					break;
				case "denied":
					clients = [];
					break;
			}

			// Sort by name
			clients.sort((a, b) =>
				a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
			);

			const dtos = clients.map(toMyClientResponse);

			return jsonSuccess(reply, {
				clients: dtos,
				total: dtos.length,
			});
		},
	);

	// GET /api/me/clients/:clientId - Get specific accessible client
	fastify.get(
		"/clients/:clientId",
		{
			preHandler: requireAuthentication(),
			schema: {
				params: ClientIdParam,
				response: {
					200: MyClientSchema,
					401: ErrorResponseSchema,
					403: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { clientId } = request.params as Static<typeof ClientIdParam>;
			const principal = request.audit.principal!;

			// Resource-level authorization
			if (!canAccessClient(principal, clientId)) {
				return forbidden(reply, "You do not have access to this client");
			}

			const client = await clientRepository.findById(clientId);
			if (!client) {
				return notFound(reply, `Client not found: ${clientId}`);
			}

			return jsonSuccess(reply, toMyClientResponse(client));
		},
	);

	// GET /api/me/clients/:clientId/applications - Get applications for an accessible client
	fastify.get(
		"/clients/:clientId/applications",
		{
			preHandler: requireAuthentication(),
			schema: {
				params: ClientIdParam,
				response: {
					200: MyApplicationsResponseSchema,
					401: ErrorResponseSchema,
					403: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { clientId } = request.params as Static<typeof ClientIdParam>;
			const principal = request.audit.principal!;

			// Resource-level authorization
			if (!canAccessClient(principal, clientId)) {
				return forbidden(reply, "You do not have access to this client");
			}

			// Verify client exists
			const client = await clientRepository.findById(clientId);
			if (!client) {
				return notFound(reply, `Client not found: ${clientId}`);
			}

			// Get enabled application configs for this client
			const configs =
				await applicationClientConfigRepository.findByClient(clientId);
			const enabledConfigsByAppId = new Map(
				configs.filter((c) => c.enabled).map((c) => [c.applicationId, c]),
			);

			if (enabledConfigsByAppId.size === 0) {
				return jsonSuccess(reply, {
					applications: [],
					total: 0,
					clientId,
				});
			}

			// Get all applications and filter to enabled+active ones
			const allApps = await applicationRepository.findAll();
			const enabledApps = allApps.filter(
				(app) => app.active && enabledConfigsByAppId.has(app.id),
			);

			// Build response with effective URLs
			const dtos: MyApplicationResponse[] = enabledApps
				.map((app) => ({
					id: app.id,
					code: app.code,
					name: app.name,
					description: app.description,
					iconUrl: app.iconUrl,
					baseUrl: app.defaultBaseUrl,
					website: app.website,
					logoMimeType: app.logoMimeType,
				}))
				.sort((a, b) =>
					a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
				);

			return jsonSuccess(reply, {
				applications: dtos,
				total: dtos.length,
				clientId,
			});
		},
	);
}

// ─── Mappers ────────────────────────────────────────────────────────────────

function toMyClientResponse(client: {
	id: string;
	name: string;
	identifier: string;
	status: string;
	createdAt: Date;
	updatedAt: Date;
}): MyClientResponse {
	return {
		id: client.id,
		name: client.name,
		identifier: client.identifier,
		status: client.status,
		createdAt: client.createdAt.toISOString(),
		updatedAt: client.updatedAt.toISOString(),
	};
}
