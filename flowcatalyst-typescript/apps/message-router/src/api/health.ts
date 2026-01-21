import { createRoute, OpenAPIHono, z } from '@hono/zod-openapi';
import { HealthCheckResponseSchema } from '@flowcatalyst/shared-types';
import type { AppContext } from '../app.js';

export const healthRoutes = new OpenAPIHono<AppContext>();

/**
 * Health check response schema for OpenAPI
 */
const HealthResponseSchema = z.object({
	status: z.string(),
	timestamp: z.string(),
	issues: z.array(z.string()),
});

/**
 * GET /health/live - Kubernetes liveness probe
 */
const liveRoute = createRoute({
	method: 'get',
	path: '/live',
	tags: ['Health'],
	summary: 'Liveness probe',
	description: 'Check if the application is alive',
	responses: {
		200: {
			description: 'Application is alive',
			content: {
				'application/json': {
					schema: HealthResponseSchema,
				},
			},
		},
		503: {
			description: 'Application is not alive',
			content: {
				'application/json': {
					schema: HealthResponseSchema,
				},
			},
		},
	},
});

healthRoutes.openapi(liveRoute, (c) => {
	const services = c.get('services');
	const health = services.health.getLiveness();

	const response = {
		status: health.healthy ? 'ALIVE' : 'NOT_ALIVE',
		timestamp: new Date().toISOString(),
		issues: health.issues,
	};

	return c.json(response, health.healthy ? 200 : 503);
});

/**
 * GET /health/ready - Kubernetes readiness probe
 */
const readyRoute = createRoute({
	method: 'get',
	path: '/ready',
	tags: ['Health'],
	summary: 'Readiness probe',
	description: 'Check if the application is ready to accept traffic',
	responses: {
		200: {
			description: 'Application is ready',
			content: {
				'application/json': {
					schema: HealthResponseSchema,
				},
			},
		},
		503: {
			description: 'Application is not ready',
			content: {
				'application/json': {
					schema: HealthResponseSchema,
				},
			},
		},
	},
});

healthRoutes.openapi(readyRoute, async (c) => {
	const services = c.get('services');
	const health = await services.health.getReadiness();

	const response = {
		status: health.healthy ? 'READY' : 'NOT_READY',
		timestamp: new Date().toISOString(),
		issues: health.issues,
	};

	return c.json(response, health.healthy ? 200 : 503);
});

/**
 * GET /health/startup - Kubernetes startup probe
 */
const startupRoute = createRoute({
	method: 'get',
	path: '/startup',
	tags: ['Health'],
	summary: 'Startup probe',
	description: 'Check if the application has started',
	responses: {
		200: {
			description: 'Application has started',
			content: {
				'application/json': {
					schema: HealthResponseSchema,
				},
			},
		},
		503: {
			description: 'Application has not started',
			content: {
				'application/json': {
					schema: HealthResponseSchema,
				},
			},
		},
	},
});

healthRoutes.openapi(startupRoute, (c) => {
	const services = c.get('services');
	const health = services.health.getStartup();

	const response = {
		status: health.healthy ? 'READY' : 'NOT_READY',
		timestamp: new Date().toISOString(),
		issues: health.issues,
	};

	return c.json(response, health.healthy ? 200 : 503);
});
