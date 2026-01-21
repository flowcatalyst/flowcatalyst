import { createRoute, OpenAPIHono, z } from '@hono/zod-openapi';
import { SeedMessageRequestSchema, SeedMessageResponseSchema } from '@flowcatalyst/shared-types';
import type { AppContext } from '../app.js';

export const seedRoutes = new OpenAPIHono<AppContext>();

/**
 * POST /api/seed/messages - Seed messages to queue
 */
const seedMessagesRoute = createRoute({
	method: 'post',
	path: '/messages',
	tags: ['Seed'],
	summary: 'Seed messages to queue',
	description: 'Generate test messages and send to queue for processing',
	request: {
		body: {
			content: {
				'application/json': {
					schema: SeedMessageRequestSchema,
				},
			},
		},
	},
	responses: {
		200: {
			description: 'Messages seeded successfully',
			content: {
				'application/json': {
					schema: SeedMessageResponseSchema,
				},
			},
		},
		500: {
			description: 'Seeding failed',
			content: {
				'application/json': {
					schema: SeedMessageResponseSchema,
				},
			},
		},
	},
});

seedRoutes.openapi(seedMessagesRoute, async (c) => {
	const services = c.get('services');
	const logger = c.get('logger');
	const body = c.req.valid('json');

	const {
		count = 10,
		queue = 'random',
		endpoint = 'random',
		messageGroupMode = '1of8',
	} = body;

	try {
		const result = await services.seeder.seedMessages({
			count,
			queue,
			endpoint,
			messageGroupMode,
		});

		return c.json({
			status: 'success',
			messagesSent: result.messagesSent,
			totalRequested: count,
		});
	} catch (error) {
		logger.error({ err: error }, 'Failed to seed messages');
		return c.json(
			{
				status: 'error',
				message: error instanceof Error ? error.message : 'Unknown error',
			},
			500,
		);
	}
});
