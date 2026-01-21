import { createRoute, OpenAPIHono, z } from '@hono/zod-openapi';
import type { AppContext } from '../app.js';

/**
 * Benchmark state - tracks request counts and timing
 */
let requestCount = 0;
let startTime = 0;

/**
 * Benchmark routes for measuring message router throughput
 */
export const benchmarkRoutes = new OpenAPIHono<AppContext>();

/**
 * Response schemas
 */
const BenchmarkProcessResponseSchema = z.object({
	status: z.string(),
	requestId: z.number(),
	timestamp: z.number(),
});

const BenchmarkProcessSlowResponseSchema = z.object({
	status: z.string(),
	requestId: z.number(),
	delayMs: z.number(),
	timestamp: z.number(),
});

const BenchmarkStatsResponseSchema = z.object({
	totalRequests: z.number(),
	elapsedMs: z.number(),
	throughputPerSecond: z.number(),
});

const BenchmarkResetResponseSchema = z.object({
	status: z.string(),
});

/**
 * POST /benchmark/process - Fast mock endpoint for throughput testing
 */
const processRoute = createRoute({
	method: 'post',
	path: '/process',
	tags: ['Benchmark'],
	summary: 'Fast mock processing endpoint',
	description: 'Returns immediate 200 OK response for measuring pure routing performance',
	request: {
		body: {
			content: {
				'application/json': {
					schema: z.record(z.unknown()).optional(),
				},
			},
		},
	},
	responses: {
		200: {
			description: 'Processing complete',
			content: {
				'application/json': {
					schema: BenchmarkProcessResponseSchema,
				},
			},
		},
	},
});

benchmarkRoutes.openapi(processRoute, (c) => {
	requestCount++;
	const count = requestCount;

	// Initialize start time on first request
	if (count === 1) {
		startTime = Date.now();
	}

	// Log every 100 requests
	if (count % 100 === 0) {
		const elapsed = Date.now() - startTime;
		const throughput = count / (elapsed / 1000);
		const logger = c.get('logger');
		logger.info({ count, throughput: throughput.toFixed(2) }, 'Benchmark progress');
	}

	return c.json({
		status: 'ok',
		requestId: count,
		timestamp: Date.now(),
	});
});

/**
 * POST /benchmark/process-slow - Mock endpoint with simulated delay
 */
const processSlowRoute = createRoute({
	method: 'post',
	path: '/process-slow',
	tags: ['Benchmark'],
	summary: 'Slow mock processing endpoint',
	description: 'Returns 200 OK after configurable delay for testing with latency',
	request: {
		query: z.object({
			delayMs: z.string().transform(Number).default('100'),
		}),
		body: {
			content: {
				'application/json': {
					schema: z.record(z.unknown()).optional(),
				},
			},
		},
	},
	responses: {
		200: {
			description: 'Processing complete',
			content: {
				'application/json': {
					schema: BenchmarkProcessSlowResponseSchema,
				},
			},
		},
	},
});

benchmarkRoutes.openapi(processSlowRoute, async (c) => {
	const { delayMs } = c.req.valid('query');

	// Simulate processing delay
	await new Promise((resolve) => setTimeout(resolve, delayMs));

	requestCount++;
	const count = requestCount;

	return c.json({
		status: 'ok',
		requestId: count,
		delayMs,
		timestamp: Date.now(),
	});
});

/**
 * GET /benchmark/stats - Get current benchmark statistics
 */
const statsRoute = createRoute({
	method: 'get',
	path: '/stats',
	tags: ['Benchmark'],
	summary: 'Get benchmark statistics',
	description: 'Returns total requests, elapsed time, and throughput',
	responses: {
		200: {
			description: 'Benchmark statistics',
			content: {
				'application/json': {
					schema: BenchmarkStatsResponseSchema,
				},
			},
		},
	},
});

benchmarkRoutes.openapi(statsRoute, (c) => {
	const elapsed = startTime > 0 ? Date.now() - startTime : 0;
	const throughput = startTime > 0 && elapsed > 0 ? requestCount / (elapsed / 1000) : 0;

	return c.json({
		totalRequests: requestCount,
		elapsedMs: elapsed,
		throughputPerSecond: Math.round(throughput * 100) / 100,
	});
});

/**
 * POST /benchmark/reset - Reset benchmark statistics
 */
const resetRoute = createRoute({
	method: 'post',
	path: '/reset',
	tags: ['Benchmark'],
	summary: 'Reset benchmark statistics',
	description: 'Clears all counters and timers',
	responses: {
		200: {
			description: 'Stats reset',
			content: {
				'application/json': {
					schema: BenchmarkResetResponseSchema,
				},
			},
		},
	},
});

benchmarkRoutes.openapi(resetRoute, (c) => {
	requestCount = 0;
	startTime = 0;

	const logger = c.get('logger');
	logger.info('Benchmark stats reset');

	return c.json({ status: 'reset' });
});
