/**
 * Dispatch Jobs Read Schema
 *
 * Read-optimized projection of dispatch jobs for efficient querying.
 * Populated by the stream processor from dispatch_job_projection_feed.
 *
 * Key differences from dispatch_jobs table:
 * - No payload/headers/metadata (stored in normalized tables)
 * - Parsed application/subdomain/aggregate fields for filtering
 * - isCompleted/isTerminal flags for quick status checks
 * - projected_at timestamp for tracking projection lag
 */

import { pgTable, varchar, text, integer, bigint, boolean, index } from 'drizzle-orm/pg-core';
import { tsidColumn, rawTsidColumn, timestampColumn } from './common.js';
import type {
  DispatchKind,
  DispatchStatus,
  DispatchMode,
  DispatchProtocol,
} from './dispatch-jobs.js';

/**
 * Dispatch jobs read table schema.
 */
export const dispatchJobsRead = pgTable(
  'dispatch_jobs_read',
  {
    // Primary key (same as source dispatch_job.id, unprefixed for performance)
    id: rawTsidColumn('id').primaryKey(),

    // External reference
    externalId: varchar('external_id', { length: 100 }),

    // Classification
    source: varchar('source', { length: 500 }),
    kind: varchar('kind', { length: 20 }).notNull().$type<DispatchKind>(),
    code: varchar('code', { length: 200 }).notNull(),
    subject: varchar('subject', { length: 500 }),

    // Event reference (unprefixed - events use raw TSIDs)
    eventId: rawTsidColumn('event_id'),
    correlationId: varchar('correlation_id', { length: 100 }),

    // Target (URL only, no payload)
    targetUrl: varchar('target_url', { length: 500 }).notNull(),
    protocol: varchar('protocol', { length: 30 }).notNull().$type<DispatchProtocol>(),

    // References
    serviceAccountId: tsidColumn('service_account_id'),
    clientId: tsidColumn('client_id'),
    subscriptionId: tsidColumn('subscription_id'),
    dispatchPoolId: tsidColumn('dispatch_pool_id'),

    // Behavior
    mode: varchar('mode', { length: 30 }).notNull().$type<DispatchMode>(),
    messageGroup: varchar('message_group', { length: 200 }),
    sequence: integer('sequence').default(99),
    timeoutSeconds: integer('timeout_seconds').default(30),

    // Execution state
    status: varchar('status', { length: 20 }).notNull().$type<DispatchStatus>(),
    maxRetries: integer('max_retries').notNull(),
    retryStrategy: varchar('retry_strategy', { length: 50 }),
    scheduledFor: timestampColumn('scheduled_for'),
    expiresAt: timestampColumn('expires_at'),

    // Tracking
    attemptCount: integer('attempt_count').notNull().default(0),
    lastAttemptAt: timestampColumn('last_attempt_at'),
    completedAt: timestampColumn('completed_at'),
    durationMillis: bigint('duration_millis', { mode: 'number' }),
    lastError: text('last_error'),
    idempotencyKey: varchar('idempotency_key', { length: 100 }),

    // Computed flags for efficient queries
    isCompleted: boolean('is_completed'),
    isTerminal: boolean('is_terminal'),

    // Parsed fields for efficient filtering (extracted from code/subject)
    application: varchar('application', { length: 100 }),
    subdomain: varchar('subdomain', { length: 100 }),
    aggregate: varchar('aggregate', { length: 100 }),

    // Timestamps
    createdAt: timestampColumn('created_at').notNull(),
    updatedAt: timestampColumn('updated_at').notNull(),
    projectedAt: timestampColumn('projected_at'),
  },
  (table) => [
    index('idx_dispatch_jobs_read_status').on(table.status),
    index('idx_dispatch_jobs_read_client_id').on(table.clientId),
    index('idx_dispatch_jobs_read_application').on(table.application),
    index('idx_dispatch_jobs_read_subscription_id').on(table.subscriptionId),
    index('idx_dispatch_jobs_read_message_group').on(table.messageGroup),
    index('idx_dispatch_jobs_read_created_at').on(table.createdAt),
  ],
);

/**
 * Dispatch job read entity type (select result).
 */
export type DispatchJobReadRecord = typeof dispatchJobsRead.$inferSelect;

/**
 * New dispatch job read type (insert input).
 */
export type NewDispatchJobReadRecord = typeof dispatchJobsRead.$inferInsert;
