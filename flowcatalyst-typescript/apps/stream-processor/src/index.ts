/**
 * FlowCatalyst Stream Processor
 *
 * PostgreSQL-based linear projection services for CQRS read models.
 *
 * Runs two independent polling loops:
 *   - Event projection: event_projection_feed -> events_read
 *   - Dispatch job projection: dispatch_job_projection_feed -> dispatch_jobs_read
 *
 * Drop-in replacement for the Java flowcatalyst-stream-processor module.
 */

import postgres from 'postgres';
import { createLogger, setDefaultLogger } from '@flowcatalyst/logging';
import { env } from './env.js';
import { createEventProjectionService } from './event-projection-service.js';
import { createDispatchJobProjectionService } from './dispatch-job-projection-service.js';

/**
 * Stream processor configuration options for in-process embedding.
 */
export interface StreamProcessorConfig {
  databaseUrl?: string;
  logLevel?: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';
}

/**
 * Handle returned from startStreamProcessor for lifecycle management.
 */
export interface StreamProcessorHandle {
  stop: () => Promise<void>;
}

/**
 * Start the FlowCatalyst Stream Processor.
 *
 * @param config - Optional overrides for database URL, log level
 * @returns Handle with stop() method for graceful shutdown
 */
export async function startStreamProcessor(
  config?: StreamProcessorConfig,
): Promise<StreamProcessorHandle> {
  const DATABASE_URL = config?.databaseUrl ?? env.DATABASE_URL;
  const LOG_LEVEL = config?.logLevel ?? env.LOG_LEVEL;

  // Initialize logger
  const logger = createLogger({
    level: LOG_LEVEL,
    serviceName: 'stream-processor',
    pretty: env.NODE_ENV === 'development',
  });
  setDefaultLogger(logger);

  // Create PostgreSQL connection
  const sql = postgres(DATABASE_URL, {
    max: 4, // Two pollers + headroom
    idle_timeout: 20,
    connect_timeout: 30,
  });

  // Create projection services
  const eventProjection = createEventProjectionService(
    sql,
    {
      enabled: env.STREAM_PROCESSOR_EVENTS_ENABLED,
      batchSize: env.STREAM_PROCESSOR_EVENTS_BATCH_SIZE,
    },
    logger.child({ service: 'event-projection' }),
  );

  const dispatchJobProjection = createDispatchJobProjectionService(
    sql,
    {
      enabled: env.STREAM_PROCESSOR_DISPATCH_JOBS_ENABLED,
      batchSize: env.STREAM_PROCESSOR_DISPATCH_JOBS_BATCH_SIZE,
    },
    logger.child({ service: 'dispatch-job-projection' }),
  );

  // Start services
  if (env.STREAM_PROCESSOR_EVENTS_ENABLED) {
    eventProjection.start();
  } else {
    logger.info('Event projection service disabled');
  }

  if (env.STREAM_PROCESSOR_DISPATCH_JOBS_ENABLED) {
    dispatchJobProjection.start();
  } else {
    logger.info('Dispatch job projection service disabled');
  }

  logger.info(
    {
      eventsEnabled: env.STREAM_PROCESSOR_EVENTS_ENABLED,
      eventsBatchSize: env.STREAM_PROCESSOR_EVENTS_BATCH_SIZE,
      dispatchJobsEnabled: env.STREAM_PROCESSOR_DISPATCH_JOBS_ENABLED,
      dispatchJobsBatchSize: env.STREAM_PROCESSOR_DISPATCH_JOBS_BATCH_SIZE,
    },
    'Stream processor started',
  );

  return {
    stop: async () => {
      logger.info('Shutting down stream processor...');
      eventProjection.stop();
      dispatchJobProjection.stop();
      // Allow current polls to complete (max 2 seconds)
      await new Promise((resolve) => setTimeout(resolve, 2000));
      await sql.end();
      logger.info('Stream processor stopped');
    },
  };
}

// Run when executed as main module
const isMainModule =
  typeof process !== 'undefined' &&
  process.argv[1] &&
  (process.argv[1].endsWith('/index.ts') || process.argv[1].endsWith('/index.js'));

if (isMainModule) {
  const handle = await startStreamProcessor();

  // Graceful shutdown
  const shutdown = async () => {
    await handle.stop();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);
}
