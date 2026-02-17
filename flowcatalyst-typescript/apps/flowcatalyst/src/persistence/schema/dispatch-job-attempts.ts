/**
 * Dispatch Job Attempts Schema
 *
 * Normalized delivery attempt history for dispatch jobs.
 * Each row represents one delivery attempt with its outcome.
 */

import { pgTable, varchar, text, integer, bigint, uniqueIndex, index } from 'drizzle-orm/pg-core';
import { tsidColumn, rawTsidColumn, timestampColumn } from './common.js';

/**
 * Error type classification for failed attempts.
 */
export type DispatchErrorType = 'TIMEOUT' | 'CONNECTION' | 'HTTP_ERROR' | 'VALIDATION' | 'UNKNOWN';

/**
 * Dispatch job attempts table schema.
 */
export const dispatchJobAttempts = pgTable(
  'msg_dispatch_job_attempts',
  {
    // Primary key
    id: tsidColumn('id').primaryKey(),

    // Reference to parent dispatch job (unprefixed - dispatch_jobs use raw TSIDs)
    dispatchJobId: rawTsidColumn('dispatch_job_id').notNull(),

    // Attempt tracking
    attemptNumber: integer('attempt_number'),
    status: varchar('status', { length: 20 }),

    // Response details
    responseCode: integer('response_code'),
    responseBody: text('response_body'),
    errorMessage: text('error_message'),
    errorStackTrace: text('error_stack_trace'),
    errorType: varchar('error_type', { length: 20 }).$type<DispatchErrorType>(),

    // Timing
    durationMillis: bigint('duration_millis', { mode: 'number' }),
    attemptedAt: timestampColumn('attempted_at'),
    completedAt: timestampColumn('completed_at'),
    createdAt: timestampColumn('created_at'),
  },
  (table) => [
    // Unique constraint for job + attempt number
    uniqueIndex('idx_msg_dispatch_job_attempts_job_number').on(
      table.dispatchJobId,
      table.attemptNumber,
    ),
    // Index for finding attempts by job
    index('idx_msg_dispatch_job_attempts_job').on(table.dispatchJobId),
  ],
);

/**
 * Dispatch job attempt entity type (select result).
 */
export type DispatchJobAttemptRecord = typeof dispatchJobAttempts.$inferSelect;

/**
 * New dispatch job attempt type (insert input).
 */
export type NewDispatchJobAttemptRecord = typeof dispatchJobAttempts.$inferInsert;
