import type { FastifyPluginAsync } from 'fastify';
import { getMetrics } from '@flowcatalyst/queue-core';

export const metricsRoutes: FastifyPluginAsync = async (fastify) => {
  fastify.get('/', async (_request, reply) => {
    const metrics = getMetrics();
    const body = await metrics.getMetrics();
    return reply.type(metrics.getContentType()).send(body);
  });
};
