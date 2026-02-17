/**
 * Dispatch infrastructure — event dispatch service, SQS publisher,
 * dispatch scheduler, and unit-of-work creation.
 */

import type { FastifyBaseLogger } from 'fastify';
import {
  createTransactionManager,
  createDrizzleUnitOfWork,
  type DrizzleUnitOfWorkConfig,
  type PostCommitDispatcher,
} from '@flowcatalyst/persistence';
import type { QueuePublisher } from '@flowcatalyst/queue-core';
import type { Env } from '../env.js';
import type { Repositories } from './repositories.js';
import type { PlatformConfigService } from '../domain/index.js';
import { createEventDispatchService } from '../infrastructure/dispatch/event-dispatch-service.js';

/**
 * Build a PostCommitDispatcher from a QueuePublisher.
 * Converts DispatchJobNotification[] -> PublishMessage[] and publishes.
 */
export function createPostCommitDispatcherFromPublisher(publisher: QueuePublisher): PostCommitDispatcher {
  return {
    async dispatch(jobs) {
      if (jobs.length === 0) return;

      const messages = jobs.map((job) => ({
        messageId: job.id,
        messageGroupId: job.messageGroup,
        messageDeduplicationId: job.id,
        body: JSON.stringify({
          id: job.id,
          poolCode: job.dispatchPoolId ?? 'DEFAULT',
          messageGroupId: job.messageGroup,
        }),
      }));

      await publisher.publishBatch(messages);
    },
  };
}

export interface DispatchInfrastructure {
  uowConfig: DrizzleUnitOfWorkConfig;
  unitOfWork: ReturnType<typeof createDrizzleUnitOfWork>;
  dispatchSchedulerHandle: { stop(): void } | null;
}

export interface CreateDispatchInfrastructureDeps {
  repos: Repositories;
  aggregateRegistry: ReturnType<typeof import('@flowcatalyst/persistence').createAggregateRegistry>;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  schemaDb: any;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  db: any;
  env: Env;
  platformConfigService: PlatformConfigService;
  logger: FastifyBaseLogger;
}

export async function createDispatchInfrastructure(
  deps: CreateDispatchInfrastructureDeps,
): Promise<DispatchInfrastructure> {
  const { repos, aggregateRegistry, schemaDb, db, env, platformConfigService, logger } = deps;

  const transactionManager = createTransactionManager(schemaDb);

  // Event dispatch service (builds dispatch jobs inside UoW transaction)
  const eventDispatchService = createEventDispatchService({
    subscriptionRepository: repos.subscriptionRepository,
  });

  // Create queue publisher for post-commit dispatch (if configured)
  let postCommitDispatch: PostCommitDispatcher | undefined;

  if (env.DISPATCH_QUEUE_TYPE === 'SQS' && env.DISPATCH_QUEUE_URL) {
    const { createSqsPublisher } = await import('../../queue-core/publisher/sqs-publisher.js');
    const publisher = createSqsPublisher({
      queueUrl: env.DISPATCH_QUEUE_URL,
      region: env.DISPATCH_QUEUE_REGION,
      endpoint: env.SQS_ENDPOINT,
    });
    postCommitDispatch = createPostCommitDispatcherFromPublisher(publisher);
    logger.info({ queueUrl: env.DISPATCH_QUEUE_URL }, 'SQS post-commit dispatch configured');
  }
  // NATS, ActiveMQ, and EMBEDDED are wired externally via setPostCommitDispatcher()

  // Start Dispatch Scheduler when messaging is enabled (platform config flag)
  let dispatchSchedulerHandle: { stop(): void } | null = null;

  const messagingEnabledValue = await platformConfigService.getValue(
    'platform',
    'features',
    'messagingEnabled',
    'GLOBAL',
    null,
  );
  const messagingEnabled = messagingEnabledValue !== 'false';

  if (messagingEnabled && env.DISPATCH_QUEUE_TYPE === 'SQS' && env.DISPATCH_QUEUE_URL) {
    const { createSqsPublisher: createSchedulerPublisher } = await import(
      '../../queue-core/publisher/sqs-publisher.js'
    );
    const schedulerPublisher = createSchedulerPublisher({
      queueUrl: env.DISPATCH_QUEUE_URL,
      region: env.DISPATCH_QUEUE_REGION,
      endpoint: env.SQS_ENDPOINT,
    });
    const { startDispatchScheduler } = await import('../dispatch-scheduler/index.js');
    dispatchSchedulerHandle = startDispatchScheduler({
      db,
      publisher: schedulerPublisher,
      logger,
      config: {
        pollIntervalMs: env.DISPATCH_SCHEDULER_POLL_INTERVAL_MS,
        batchSize: env.DISPATCH_SCHEDULER_BATCH_SIZE,
        maxConcurrentGroups: env.DISPATCH_SCHEDULER_MAX_CONCURRENT_GROUPS,
        processingEndpoint: env.DISPATCH_SCHEDULER_PROCESSING_ENDPOINT,
        defaultDispatchPoolCode: env.DISPATCH_SCHEDULER_DEFAULT_POOL_CODE,
        staleQueuedThresholdMinutes: env.DISPATCH_SCHEDULER_STALE_THRESHOLD_MINUTES,
        staleQueuedPollIntervalMs: env.DISPATCH_SCHEDULER_STALE_POLL_INTERVAL_MS,
      },
    });
    logger.info('Dispatch Scheduler started (messagingEnabled=true, SQS publisher)');
  }

  // Create unit of work (postCommitDispatch is mutable — can be set later for EMBEDDED mode)
  const uowConfig: DrizzleUnitOfWorkConfig = {
    transactionManager,
    aggregateRegistry,
    extractClientId: (aggregate) => {
      // Only extract tenant client ID from entities that carry multi-tenant scoping
      // (e.g., Principal.clientId). Skip entities like OAuthClient where clientId
      // is the OAuth protocol identifier, not a tenant reference.
      if (
        'clientId' in aggregate &&
        typeof aggregate.clientId === 'string' &&
        'type' in aggregate &&
        (aggregate.type === 'USER' || aggregate.type === 'SERVICE')
      ) {
        return aggregate.clientId;
      }
      return null;
    },
    eventDispatchService,
    postCommitDispatch,
  };

  const unitOfWork = createDrizzleUnitOfWork(uowConfig);

  return { uowConfig, unitOfWork, dispatchSchedulerHandle };
}
