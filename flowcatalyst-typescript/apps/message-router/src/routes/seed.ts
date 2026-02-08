import type { FastifyPluginAsync } from 'fastify';
import { SeedMessageRequestSchema, SeedMessageResponseSchema } from '../schemas/index.js';

export const seedRoutes: FastifyPluginAsync = async (fastify) => {
	fastify.post('/messages', {
		schema: {
			tags: ['Seed'],
			summary: 'Seed messages to queue',
			body: SeedMessageRequestSchema,
			response: {
				200: SeedMessageResponseSchema,
				500: SeedMessageResponseSchema,
			},
		},
	}, async (request, reply) => {
		const {
			count = 10,
			queue = 'random',
			endpoint = 'random',
			messageGroupMode = '1of8',
		} = request.body as {
			count?: number;
			queue?: string;
			endpoint?: string;
			messageGroupMode?: string;
		};

		try {
			const result = await request.services.seeder.seedMessages({
				count,
				queue,
				endpoint,
				messageGroupMode,
			});

			return {
				status: 'success',
				messagesSent: result.messagesSent,
				totalRequested: count,
			};
		} catch (error) {
			request.log.error({ err: error }, 'Failed to seed messages');
			return reply.code(500).send({
				status: 'error',
				message: error instanceof Error ? error.message : 'Unknown error',
			});
		}
	});
};
