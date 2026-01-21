import { createRoute, OpenAPIHono, z } from '@hono/zod-openapi';
import {
	TestEndpointResponseSchema,
	MediationResponseSchema,
	TestStatsResponseSchema,
	TestStatsResetResponseSchema,
} from '@flowcatalyst/shared-types';
import type { AppContext } from '../app.js';

export const testRoutes = new OpenAPIHono<AppContext>();

// Request counter for test endpoints
let requestCounter = 0;

/**
 * POST /api/test/fast - Fast response endpoint (~100ms)
 */
const fastRoute = createRoute({
	method: 'post',
	path: '/fast',
	tags: ['Test'],
	summary: 'Fast response endpoint',
	description: 'Returns after ~100ms delay',
	responses: {
		200: {
			description: 'Success',
			content: {
				'application/json': {
					schema: TestEndpointResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(fastRoute, async (c) => {
	const requestId = ++requestCounter;
	await sleep(100);
	return c.json({
		status: 'success',
		endpoint: 'fast',
		requestId,
	});
});

/**
 * POST /api/test/slow - Slow response endpoint (~60s)
 */
const slowRoute = createRoute({
	method: 'post',
	path: '/slow',
	tags: ['Test'],
	summary: 'Slow response endpoint',
	description: 'Returns after ~60 seconds delay',
	responses: {
		200: {
			description: 'Success',
			content: {
				'application/json': {
					schema: TestEndpointResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(slowRoute, async (c) => {
	const requestId = ++requestCounter;
	await sleep(60000);
	return c.json({
		status: 'success',
		endpoint: 'slow',
		requestId,
	});
});

/**
 * POST /api/test/faulty - Faulty endpoint (60% success, 20% 400, 20% 500)
 */
const faultyRoute = createRoute({
	method: 'post',
	path: '/faulty',
	tags: ['Test'],
	summary: 'Faulty endpoint',
	description: '60% success, 20% client error, 20% server error',
	responses: {
		200: {
			description: 'Success',
			content: {
				'application/json': {
					schema: TestEndpointResponseSchema,
				},
			},
		},
		400: {
			description: 'Client error',
			content: {
				'application/json': {
					schema: TestEndpointResponseSchema,
				},
			},
		},
		500: {
			description: 'Server error',
			content: {
				'application/json': {
					schema: TestEndpointResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(faultyRoute, async (c) => {
	const requestId = ++requestCounter;
	const body = await c.req.json().catch(() => ({}));
	const messageId = (body as { messageId?: string }).messageId || 'unknown';

	const random = Math.random();

	if (random < 0.6) {
		return c.json({
			status: 'success',
			endpoint: 'faulty',
			requestId,
			messageId,
		});
	} else if (random < 0.8) {
		return c.json(
			{
				status: 'error',
				endpoint: 'faulty',
				requestId,
				messageId,
				error: 'Bad Request',
			},
			400,
		);
	} else {
		return c.json(
			{
				status: 'error',
				endpoint: 'faulty',
				requestId,
				messageId,
				error: 'Internal Server Error',
			},
			500,
		);
	}
});

/**
 * POST /api/test/fail - Always fails endpoint
 */
const failRoute = createRoute({
	method: 'post',
	path: '/fail',
	tags: ['Test'],
	summary: 'Always fails endpoint',
	responses: {
		500: {
			description: 'Server error',
			content: {
				'application/json': {
					schema: TestEndpointResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(failRoute, (c) => {
	const requestId = ++requestCounter;
	return c.json(
		{
			status: 'error',
			endpoint: 'fail',
			requestId,
			error: 'Always fails',
		},
		500,
	);
});

/**
 * POST /api/test/success - Mediation success endpoint
 */
const successRoute = createRoute({
	method: 'post',
	path: '/success',
	tags: ['Test'],
	summary: 'Mediation success endpoint',
	description: 'Returns ack=true mediation response',
	responses: {
		200: {
			description: 'Success',
			content: {
				'application/json': {
					schema: MediationResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(successRoute, (c) => {
	requestCounter++;
	return c.json({
		ack: true,
		message: '',
	});
});

/**
 * POST /api/test/pending - Mediation pending endpoint
 */
const pendingRoute = createRoute({
	method: 'post',
	path: '/pending',
	tags: ['Test'],
	summary: 'Mediation pending endpoint',
	description: 'Returns ack=false mediation response (message not ready)',
	responses: {
		200: {
			description: 'Pending',
			content: {
				'application/json': {
					schema: MediationResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(pendingRoute, (c) => {
	requestCounter++;
	return c.json({
		ack: false,
		message: 'notBefore time not reached',
	});
});

/**
 * POST /api/test/client-error - Client error endpoint
 */
const clientErrorRoute = createRoute({
	method: 'post',
	path: '/client-error',
	tags: ['Test'],
	summary: 'Client error endpoint',
	description: 'Always returns 400 Bad Request',
	responses: {
		400: {
			description: 'Client error',
			content: {
				'application/json': {
					schema: TestEndpointResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(clientErrorRoute, (c) => {
	const requestId = ++requestCounter;
	return c.json(
		{
			status: 'error',
			endpoint: 'client-error',
			requestId,
			error: 'Record not found',
		},
		400,
	);
});

/**
 * POST /api/test/server-error - Server error endpoint
 */
const serverErrorRoute = createRoute({
	method: 'post',
	path: '/server-error',
	tags: ['Test'],
	summary: 'Server error endpoint',
	description: 'Always returns 500 Internal Server Error',
	responses: {
		500: {
			description: 'Server error',
			content: {
				'application/json': {
					schema: TestEndpointResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(serverErrorRoute, (c) => {
	const requestId = ++requestCounter;
	return c.json(
		{
			status: 'error',
			endpoint: 'server-error',
			requestId,
		},
		500,
	);
});

/**
 * GET /api/test/stats - Get request stats
 */
const statsRoute = createRoute({
	method: 'get',
	path: '/stats',
	tags: ['Test'],
	summary: 'Get request statistics',
	responses: {
		200: {
			description: 'Request statistics',
			content: {
				'application/json': {
					schema: TestStatsResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(statsRoute, (c) => {
	return c.json({
		totalRequests: requestCounter,
	});
});

/**
 * POST /api/test/stats/reset - Reset request stats
 */
const resetStatsRoute = createRoute({
	method: 'post',
	path: '/stats/reset',
	tags: ['Test'],
	summary: 'Reset request statistics',
	responses: {
		200: {
			description: 'Statistics reset',
			content: {
				'application/json': {
					schema: TestStatsResetResponseSchema,
				},
			},
		},
	},
});

testRoutes.openapi(resetStatsRoute, (c) => {
	const previousCount = requestCounter;
	requestCounter = 0;
	return c.json({
		previousCount,
		currentCount: 0,
	});
});

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
