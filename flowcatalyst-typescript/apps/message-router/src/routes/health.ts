import type { FastifyPluginAsync } from 'fastify';
import { HealthCheckResponseSchema } from '../schemas/index.js';

export const healthRoutes: FastifyPluginAsync = async (fastify) => {
  fastify.get(
    '/live',
    {
      schema: {
        tags: ['Health'],
        summary: 'Liveness probe',
        response: {
          200: HealthCheckResponseSchema,
          503: HealthCheckResponseSchema,
        },
      },
    },
    (request, reply) => {
      const health = request.services.health.getLiveness();
      const response = {
        status: health.healthy ? 'ALIVE' : 'NOT_ALIVE',
        timestamp: new Date().toISOString(),
        issues: health.issues,
      };
      return reply.code(health.healthy ? 200 : 503).send(response);
    },
  );

  fastify.get(
    '/ready',
    {
      schema: {
        tags: ['Health'],
        summary: 'Readiness probe',
        response: {
          200: HealthCheckResponseSchema,
          503: HealthCheckResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const health = await request.services.health.getReadiness();
      const response = {
        status: health.healthy ? 'READY' : 'NOT_READY',
        timestamp: new Date().toISOString(),
        issues: health.issues,
      };
      return reply.code(health.healthy ? 200 : 503).send(response);
    },
  );

  fastify.get(
    '/startup',
    {
      schema: {
        tags: ['Health'],
        summary: 'Startup probe',
        response: {
          200: HealthCheckResponseSchema,
          503: HealthCheckResponseSchema,
        },
      },
    },
    (request, reply) => {
      const health = request.services.health.getStartup();
      const response = {
        status: health.healthy ? 'READY' : 'NOT_READY',
        timestamp: new Date().toISOString(),
        issues: health.issues,
      };
      return reply.code(health.healthy ? 200 : 503).send(response);
    },
  );
};
