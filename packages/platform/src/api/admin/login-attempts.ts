/**
 * Login Attempts Admin API
 *
 * REST endpoint for viewing login attempt records.
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import { jsonSuccess } from "@flowcatalyst/http";

import type { LoginAttemptRepository } from "../../infrastructure/persistence/index.js";
import { requirePermission } from "../../authorization/index.js";
import { LOGIN_ATTEMPT_PERMISSIONS } from "../../authorization/permissions/platform-admin.js";

// ─── Query Schema ─────────────────────────────────────────────────────────────

const ListLoginAttemptsQuery = Type.Object({
	attemptType: Type.Optional(Type.String()),
	outcome: Type.Optional(Type.String()),
	identifier: Type.Optional(Type.String()),
	principalId: Type.Optional(Type.String()),
	dateFrom: Type.Optional(Type.String()),
	dateTo: Type.Optional(Type.String()),
	page: Type.Optional(Type.String()),
	pageSize: Type.Optional(Type.String()),
	sortField: Type.Optional(Type.String()),
	sortOrder: Type.Optional(Type.String()),
});

// ─── Response Schemas ─────────────────────────────────────────────────────────

const LoginAttemptResponseSchema = Type.Object({
	id: Type.String(),
	attemptType: Type.String(),
	outcome: Type.String(),
	failureReason: Type.Union([Type.String(), Type.Null()]),
	identifier: Type.String(),
	principalId: Type.Union([Type.String(), Type.Null()]),
	ipAddress: Type.Union([Type.String(), Type.Null()]),
	userAgent: Type.Union([Type.String(), Type.Null()]),
	attemptedAt: Type.String({ format: "date-time" }),
});

const LoginAttemptListResponseSchema = Type.Object({
	items: Type.Array(LoginAttemptResponseSchema),
	total: Type.Integer(),
	page: Type.Integer(),
	pageSize: Type.Integer(),
});

/**
 * Dependencies for the login attempts API.
 */
export interface LoginAttemptsRoutesDeps {
	readonly loginAttemptRepository: LoginAttemptRepository;
}

/**
 * Register login attempts admin API routes.
 */
export async function registerLoginAttemptsRoutes(
	fastify: FastifyInstance,
	deps: LoginAttemptsRoutesDeps,
): Promise<void> {
	const { loginAttemptRepository } = deps;

	const DEFAULT_PAGE_SIZE = 100;
	const MAX_PAGE_SIZE = 500;

	// GET /api/admin/login-attempts - List login attempts with filters
	fastify.get(
		"/login-attempts",
		{
			preHandler: requirePermission(LOGIN_ATTEMPT_PERMISSIONS.READ),
			schema: {
				querystring: ListLoginAttemptsQuery,
				response: {
					200: LoginAttemptListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof ListLoginAttemptsQuery>;

			const page = Math.max(parseInt(query.page ?? "0", 10) || 0, 0);
			const pageSize = Math.min(
				Math.max(
					parseInt(query.pageSize ?? String(DEFAULT_PAGE_SIZE), 10) ||
						DEFAULT_PAGE_SIZE,
					1,
				),
				MAX_PAGE_SIZE,
			);

			const result = await loginAttemptRepository.findPaged(
				{
					attemptType: query.attemptType as
						| "USER_LOGIN"
						| "SERVICE_ACCOUNT_TOKEN"
						| undefined,
					outcome: query.outcome as "SUCCESS" | "FAILURE" | undefined,
					identifier: query.identifier,
					principalId: query.principalId,
					dateFrom: query.dateFrom ? new Date(query.dateFrom) : undefined,
					dateTo: query.dateTo ? new Date(query.dateTo) : undefined,
				},
				{
					page,
					pageSize,
					sortField: query.sortField,
					sortOrder: query.sortOrder,
				},
			);

			return jsonSuccess(reply, {
				items: result.items.map((a) => ({
					id: a.id,
					attemptType: a.attemptType,
					outcome: a.outcome,
					failureReason: a.failureReason,
					identifier: a.identifier,
					principalId: a.principalId,
					ipAddress: a.ipAddress,
					userAgent: a.userAgent,
					attemptedAt: a.attemptedAt.toISOString(),
				})),
				total: result.total,
				page: result.page,
				pageSize: result.pageSize,
			});
		},
	);
}
