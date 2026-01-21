import { createRoute, OpenAPIHono, z } from '@hono/zod-openapi';
import { LocalConfigResponseSchema } from '@flowcatalyst/shared-types';
import type { AppContext } from '../app.js';

export const configRoutes = new OpenAPIHono<AppContext>();

/**
 * GET /api/config - Get local configuration
 */
const getConfigRoute = createRoute({
	method: 'get',
	path: '/',
	tags: ['Configuration'],
	summary: 'Get local configuration',
	description: 'Returns the current queue and pool configuration',
	responses: {
		200: {
			description: 'Configuration retrieved successfully',
			content: {
				'application/json': {
					schema: LocalConfigResponseSchema,
				},
			},
		},
	},
});

configRoutes.openapi(getConfigRoute, (c) => {
	const services = c.get('services');
	const config = services.queueManager.getConfig();
	return c.json(config);
});
