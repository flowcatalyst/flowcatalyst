/**
 * Config Admin API
 *
 * REST endpoints for platform configuration management.
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import {
	jsonCreated,
	jsonSuccess,
	noContent,
	notFound,
	forbidden,
	badRequest,
	ErrorResponseSchema,
} from "@flowcatalyst/http";
import type {
	PlatformConfigService,
	ConfigScope,
	ConfigValueType,
} from "../../domain/index.js";

// ─── Request Schemas ────────────────────────────────────────────────────────

const SetConfigSchema = Type.Object({
	value: Type.String(),
	valueType: Type.Optional(
		Type.Union([Type.Literal("PLAIN"), Type.Literal("SECRET")]),
	),
	description: Type.Optional(Type.String()),
});

const AppCodeParam = Type.Object({ appCode: Type.String() });
const SectionParam = Type.Object({
	appCode: Type.String(),
	section: Type.String(),
});
const PropertyParam = Type.Object({
	appCode: Type.String(),
	section: Type.String(),
	property: Type.String(),
});

const ScopeQuery = Type.Object({
	scope: Type.Optional(
		Type.Union([Type.Literal("GLOBAL"), Type.Literal("CLIENT")]),
	),
	clientId: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const ConfigResponseSchema = Type.Object({
	id: Type.String(),
	applicationCode: Type.String(),
	section: Type.String(),
	property: Type.String(),
	scope: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
	valueType: Type.String(),
	value: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	createdAt: Type.String({ format: "date-time" }),
	updatedAt: Type.String({ format: "date-time" }),
});

const ConfigListResponseSchema = Type.Object({
	items: Type.Array(ConfigResponseSchema),
});

const SectionResponseSchema = Type.Object({
	applicationCode: Type.String(),
	section: Type.String(),
	scope: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
	values: Type.Record(Type.String(), Type.String()),
});

const ValueResponseSchema = Type.Object({
	applicationCode: Type.String(),
	section: Type.String(),
	property: Type.String(),
	scope: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
	value: Type.String(),
});

/**
 * Dependencies for the config admin API.
 */
export interface ConfigRoutesDeps {
	readonly platformConfigService: PlatformConfigService;
}

/**
 * Register config admin API routes.
 */
function getRoles(request: {
	audit?: { principal?: { roles?: ReadonlySet<string> } | null };
}): readonly string[] {
	const roles = request.audit?.principal?.roles;
	return roles ? Array.from(roles) : [];
}

export async function registerConfigRoutes(
	fastify: FastifyInstance,
	deps: ConfigRoutesDeps,
): Promise<void> {
	const { platformConfigService } = deps;

	function parseScope(query: Static<typeof ScopeQuery>): {
		scope: ConfigScope;
		clientId: string | null;
	} {
		const scope = (query.scope ?? "GLOBAL") as ConfigScope;
		const clientId = query.clientId ?? null;
		return { scope, clientId };
	}

	// GET /api/admin/config/:appCode - List configs for application
	fastify.get(
		"/config/:appCode",
		{
			schema: {
				params: AppCodeParam,
				querystring: ScopeQuery,
				response: {
					200: ConfigListResponseSchema,
					403: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode } = request.params as Static<typeof AppCodeParam>;
			const query = request.query as Static<typeof ScopeQuery>;
			const { scope, clientId } = parseScope(query);
			const roles = getRoles(request);

			const hasAccess = await platformConfigService.canAccess(
				appCode,
				roles,
				false,
			);
			if (!hasAccess) {
				return forbidden(reply, "Access denied to configuration");
			}

			const configs = await platformConfigService.getConfigs(
				appCode,
				scope,
				clientId,
			);

			return jsonSuccess(reply, {
				items: configs.map(toConfigResponse),
			});
		},
	);

	// GET /api/admin/config/:appCode/:section - List section as property map
	fastify.get(
		"/config/:appCode/:section",
		{
			schema: {
				params: SectionParam,
				querystring: ScopeQuery,
				response: {
					200: SectionResponseSchema,
					403: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode, section } = request.params as Static<
				typeof SectionParam
			>;
			const query = request.query as Static<typeof ScopeQuery>;
			const { scope, clientId } = parseScope(query);
			const roles = getRoles(request);

			const hasAccess = await platformConfigService.canAccess(
				appCode,
				roles,
				false,
			);
			if (!hasAccess) {
				return forbidden(reply, "Access denied to configuration");
			}

			const values = await platformConfigService.getSection(
				appCode,
				section,
				scope,
				clientId,
			);

			return jsonSuccess(reply, {
				applicationCode: appCode,
				section,
				scope,
				clientId,
				values: Object.fromEntries(values),
			});
		},
	);

	// GET /api/admin/config/:appCode/:section/:property - Get single value
	fastify.get(
		"/config/:appCode/:section/:property",
		{
			schema: {
				params: PropertyParam,
				querystring: ScopeQuery,
				response: {
					200: ValueResponseSchema,
					403: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode, section, property } = request.params as Static<
				typeof PropertyParam
			>;
			const query = request.query as Static<typeof ScopeQuery>;
			const { scope, clientId } = parseScope(query);
			const roles = getRoles(request);

			const hasAccess = await platformConfigService.canAccess(
				appCode,
				roles,
				false,
			);
			if (!hasAccess) {
				return forbidden(reply, "Access denied to configuration");
			}

			const value = await platformConfigService.getValue(
				appCode,
				section,
				property,
				scope,
				clientId,
			);
			if (value === undefined) {
				return notFound(
					reply,
					`Config not found: ${appCode}.${section}.${property}`,
				);
			}

			return jsonSuccess(reply, {
				applicationCode: appCode,
				section,
				property,
				scope,
				clientId,
				value,
			});
		},
	);

	// PUT /api/admin/config/:appCode/:section/:property - Set config value
	fastify.put(
		"/config/:appCode/:section/:property",
		{
			schema: {
				params: PropertyParam,
				querystring: ScopeQuery,
				body: SetConfigSchema,
				response: {
					200: ConfigResponseSchema,
					201: ConfigResponseSchema,
					400: ErrorResponseSchema,
					403: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode, section, property } = request.params as Static<
				typeof PropertyParam
			>;
			const query = request.query as Static<typeof ScopeQuery>;
			const body = request.body as Static<typeof SetConfigSchema>;
			const { scope, clientId } = parseScope(query);
			const roles = getRoles(request);

			const hasAccess = await platformConfigService.canAccess(
				appCode,
				roles,
				true,
			);
			if (!hasAccess) {
				return forbidden(reply, "Write access denied to configuration");
			}

			// Validate scope/clientId consistency
			if (scope === "CLIENT" && !clientId) {
				return badRequest(reply, "clientId is required for CLIENT scope");
			}
			if (scope === "GLOBAL" && clientId) {
				return badRequest(
					reply,
					"clientId must not be provided for GLOBAL scope",
				);
			}

			// Check if this is a create or update
			const existing = await platformConfigService.getValue(
				appCode,
				section,
				property,
				scope,
				clientId,
			);
			const isCreate = existing === undefined;

			const config = await platformConfigService.setValue({
				applicationCode: appCode,
				section,
				property,
				scope,
				clientId,
				value: body.value,
				valueType: (body.valueType ?? "PLAIN") as ConfigValueType,
				description: body.description ?? null,
			});

			const response = toConfigResponse(config);

			if (isCreate) {
				return jsonCreated(reply, response);
			}
			return jsonSuccess(reply, response);
		},
	);

	// DELETE /api/admin/config/:appCode/:section/:property - Delete config
	fastify.delete(
		"/config/:appCode/:section/:property",
		{
			schema: {
				params: PropertyParam,
				querystring: ScopeQuery,
				response: {
					204: Type.Null(),
					403: ErrorResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { appCode, section, property } = request.params as Static<
				typeof PropertyParam
			>;
			const query = request.query as Static<typeof ScopeQuery>;
			const { scope, clientId } = parseScope(query);
			const roles = getRoles(request);

			const hasAccess = await platformConfigService.canAccess(
				appCode,
				roles,
				true,
			);
			if (!hasAccess) {
				return forbidden(reply, "Write access denied to configuration");
			}

			const deleted = await platformConfigService.deleteValue(
				appCode,
				section,
				property,
				scope,
				clientId,
			);
			if (!deleted) {
				return notFound(
					reply,
					`Config not found: ${appCode}.${section}.${property}`,
				);
			}

			return noContent(reply);
		},
	);
}

function toConfigResponse(config: {
	id: string;
	applicationCode: string;
	section: string;
	property: string;
	scope: string;
	clientId: string | null;
	valueType: string;
	value: string;
	description: string | null;
	createdAt: Date;
	updatedAt: Date;
}) {
	return {
		id: config.id,
		applicationCode: config.applicationCode,
		section: config.section,
		property: config.property,
		scope: config.scope,
		clientId: config.clientId,
		valueType: config.valueType,
		value: config.valueType === "SECRET" ? "***" : config.value,
		description: config.description,
		createdAt: config.createdAt.toISOString(),
		updatedAt: config.updatedAt.toISOString(),
	};
}
