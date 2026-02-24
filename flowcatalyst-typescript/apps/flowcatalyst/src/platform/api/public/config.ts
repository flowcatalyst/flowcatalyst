/**
 * Public Config API
 *
 * Unauthenticated endpoints for public platform configuration.
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import { jsonSuccess } from "@flowcatalyst/http";
import type { PlatformConfigService } from "../../domain/index.js";

// ─── Response Schemas ───────────────────────────────────────────────────────

const LoginThemeResponseSchema = Type.Object({
	brandName: Type.Union([Type.String(), Type.Null()]),
	brandSubtitle: Type.Union([Type.String(), Type.Null()]),
	logoUrl: Type.Union([Type.String(), Type.Null()]),
	logoSvg: Type.Union([Type.String(), Type.Null()]),
	logoHeight: Type.Union([Type.Integer(), Type.Null()]),
	primaryColor: Type.Union([Type.String(), Type.Null()]),
	accentColor: Type.Union([Type.String(), Type.Null()]),
	backgroundColor: Type.Union([Type.String(), Type.Null()]),
	backgroundGradient: Type.Union([Type.String(), Type.Null()]),
	footerText: Type.Union([Type.String(), Type.Null()]),
	customCss: Type.Union([Type.String(), Type.Null()]),
});

const ThemeQuery = Type.Object({
	clientId: Type.Optional(Type.String()),
});

const PlatformConfigResponseSchema = Type.Object({
	features: Type.Object({
		messagingEnabled: Type.Boolean(),
	}),
});

/**
 * Dependencies for public config routes.
 */
export interface PublicConfigRoutesDeps {
	readonly platformConfigService: PlatformConfigService;
}

/**
 * Register public config routes (no auth required).
 */
export async function registerPublicConfigRoutes(
	fastify: FastifyInstance,
	deps: PublicConfigRoutesDeps,
): Promise<void> {
	const { platformConfigService } = deps;

	const THEME_PROPERTIES = [
		"brandName",
		"brandSubtitle",
		"logoUrl",
		"logoSvg",
		"logoHeight",
		"primaryColor",
		"accentColor",
		"backgroundColor",
		"backgroundGradient",
		"footerText",
		"customCss",
	] as const;

	// GET /api/public/login-theme - Get login page theme
	fastify.get(
		"/login-theme",
		{
			schema: {
				querystring: ThemeQuery,
				response: {
					200: LoginThemeResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof ThemeQuery>;
			const clientId = query.clientId;

			let themeValues: Map<string, string>;

			if (clientId) {
				themeValues = await platformConfigService.getSectionWithFallback(
					"platform",
					"login",
					clientId,
				);
			} else {
				themeValues = await platformConfigService.getSection(
					"platform",
					"login",
					"GLOBAL",
					null,
				);
			}

			// Support both storage formats:
			// 1. Single JSON blob at property "theme" (how the settings page saves)
			// 2. Individual properties (e.g. "brandName", "primaryColor")
			let jsonBlob: Record<string, unknown> = {};
			const themeJson = themeValues.get("theme");
			if (themeJson) {
				try {
					jsonBlob = JSON.parse(themeJson);
				} catch {
					// Invalid JSON — ignore
				}
			}

			const theme: Record<string, string | number | null> = {};
			for (const prop of THEME_PROPERTIES) {
				// Individual property takes precedence over JSON blob
				const value =
					themeValues.get(prop) ??
					(jsonBlob[prop] != null ? String(jsonBlob[prop]) : null);
				if (prop === "logoHeight" && value !== null) {
					theme[prop] = parseInt(value, 10) || null;
				} else {
					theme[prop] = value;
				}
			}

			return jsonSuccess(reply, theme);
		},
	);

	// GET /api/config/platform - Get platform feature flags
	fastify.get(
		"/platform",
		{
			schema: {
				response: {
					200: PlatformConfigResponseSchema,
				},
			},
		},
		async (_request, reply) => {
			const messagingEnabled = await platformConfigService.getValue(
				"platform",
				"features",
				"messagingEnabled",
				"GLOBAL",
				null,
			);

			return jsonSuccess(reply, {
				features: {
					// Default to true when no config value is stored
					messagingEnabled: messagingEnabled !== "false",
				},
			});
		},
	);
}
