import { createLogger, setDefaultLogger } from '@flowcatalyst/logging';
import type { FastifyInstance } from 'fastify';
import type { Services } from './services/index.js';
import { env } from './env.js';
import { createApp } from './app.js';

/**
 * Router configuration options for in-process embedding.
 */
export interface RouterConfig {
  port?: number;
  host?: string;
  logLevel?: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';
}

/**
 * Start the FlowCatalyst Message Router service.
 *
 * @param config - Optional overrides for port, host, log level
 * @returns The Fastify server instance and services (for shutdown coordination)
 */
export async function startRouter(
  config?: RouterConfig,
): Promise<{ server: FastifyInstance; services: Services }> {
  const PORT = config?.port ?? env.PORT;
  const HOST = config?.host ?? env.HOST;
  const LOG_LEVEL = config?.logLevel ?? env.LOG_LEVEL;

  // Initialize logger
  const logger = createLogger({
    level: LOG_LEVEL,
    serviceName: 'message-router',
    pretty: env.NODE_ENV === 'development',
    base: {
      instanceId: env.INSTANCE_ID,
    },
  });
  setDefaultLogger(logger);

  // Create Fastify app
  const { app, services } = await createApp(logger);

  // Start server
  await app.listen({ port: PORT, host: HOST });

  logger.info(
    {
      host: HOST,
      port: PORT,
      env: env.NODE_ENV,
      queueType: env.QUEUE_TYPE,
    },
    'Message router started',
  );

  return { server: app, services };
}

// Run when executed as main module
const isMainModule =
  typeof process !== 'undefined' &&
  process.argv[1] &&
  (process.argv[1].endsWith('/index.ts') || process.argv[1].endsWith('/index.js'));

if (isMainModule) {
  const logger = createLogger({
    level: env.LOG_LEVEL,
    serviceName: 'message-router',
    pretty: env.NODE_ENV === 'development',
    base: { instanceId: env.INSTANCE_ID },
  });

  const { server, services } = await startRouter();

  // Graceful shutdown
  let shuttingDown = false;
  const shutdown = async (signal: string) => {
    if (shuttingDown) return;
    shuttingDown = true;

    logger.info({ signal }, 'Shutdown signal received');

    // Safety timeout so shutdown doesn't hang forever
    const forceShutdown = setTimeout(() => {
      logger.error('Forced shutdown after timeout');
      process.exit(1);
    }, 45_000); // 30s pool drain + 15s buffer
    forceShutdown.unref();

    // 1. Stop accepting HTTP requests
    await server.close();

    // 2. Stop health monitoring services
    services.brokerHealth.stop();
    services.queueHealthMonitor.stop();
    services.notifications.stop();

    // 3. Stop queue manager (drains pools, NACKs in-flight, stops consumers)
    await services.queueManager.stop();

    logger.info('Graceful shutdown complete');
    process.exit(0);
  };

  process.on('SIGINT', () => shutdown('SIGINT'));
  process.on('SIGTERM', () => shutdown('SIGTERM'));
}
