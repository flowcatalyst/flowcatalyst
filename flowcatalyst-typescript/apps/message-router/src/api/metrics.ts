import { OpenAPIHono } from '@hono/zod-openapi';
import { getMetrics } from '@flowcatalyst/queue-core';
import type { AppContext } from '../app.js';

export const metricsRoutes = new OpenAPIHono<AppContext>();

/**
 * GET /metrics - Prometheus metrics endpoint
 */
metricsRoutes.get('/', async (c) => {
	const metrics = getMetrics();
	const body = await metrics.getMetrics();
	return c.text(body, 200, {
		'Content-Type': metrics.getContentType(),
	});
});
