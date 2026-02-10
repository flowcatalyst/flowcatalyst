/**
 * Services Plugin
 *
 * Fastify plugin that decorates the request with application services.
 */

import type { FastifyPluginAsync } from 'fastify';
import fp from 'fastify-plugin';
import type { Services } from '../services/index.js';

export interface ServicesPluginOptions {
  services: Services;
}

declare module 'fastify' {
  interface FastifyRequest {
    services: Services;
  }
}

const servicesPluginAsync: FastifyPluginAsync<ServicesPluginOptions> = async (fastify, opts) => {
  const { services } = opts;

  fastify.decorateRequest('services', {
    getter() {
      return services;
    },
  });
};

export const servicesPlugin = fp(servicesPluginAsync, {
  name: '@flowcatalyst/message-router-services',
  fastify: '5.x',
});
