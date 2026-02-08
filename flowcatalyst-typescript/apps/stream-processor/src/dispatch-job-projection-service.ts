/**
 * Dispatch Job Projection Service
 *
 * Projects dispatch job changes from dispatch_job_projection_feed to dispatch_jobs_read.
 *
 * Uses a single writable CTE per poll cycle: one atomic statement that
 * selects the batch, applies INSERTs and UPDATEs to dispatch_jobs_read,
 * and marks feed entries as processed. Zero application-layer data
 * transfer - everything stays in PostgreSQL.
 *
 * Algorithm:
 *   1. Single CTE: batch SELECT -> INSERT projection -> UPDATE projection -> mark processed
 *   2. Sleep: 0ms if full batch, 100ms if partial, 1000ms if zero results
 *
 * Code hierarchy parsing (same as event type):
 *   code: "orders:fulfillment:shipment"
 *   -> application: "orders", subdomain: "fulfillment", aggregate: "shipment"
 */

import type postgres from 'postgres';
import type { Logger } from '@flowcatalyst/logging';

export interface DispatchJobProjectionConfig {
	readonly enabled: boolean;
	readonly batchSize: number;
}

export interface DispatchJobProjectionService {
	start(): void;
	stop(): void;
	isRunning(): boolean;
}

export function createDispatchJobProjectionService(
	sql: postgres.Sql,
	config: DispatchJobProjectionConfig,
	logger: Logger,
): DispatchJobProjectionService {
	let running = false;

	/**
	 * Single-statement batch projection using writable CTE.
	 *
	 * 1. `batch` CTE: selects unprocessed feed entries (LIMIT batchSize)
	 * 2. `projected_inserts` CTE: UPSERTs new dispatch jobs from INSERT entries
	 * 3. `projected_updates` CTE: patches existing dispatch jobs from UPDATE entries
	 * 4. Main UPDATE: marks batch entries as processed
	 *
	 * All four operations execute atomically in one round-trip.
	 * Data never leaves PostgreSQL - JSONB extraction happens in-engine.
	 *
	 * For UPDATEs with duplicate dispatch_job_ids in the same batch,
	 * DISTINCT ON (dispatch_job_id ... ORDER BY id DESC) takes the latest
	 * entry per job. COALESCE ensures only non-null patch fields are applied.
	 */
	async function pollAndProject(): Promise<number> {
		const result = await sql`
			WITH batch AS (
				SELECT id, dispatch_job_id, operation, payload
				FROM dispatch_job_projection_feed
				WHERE processed = 0
				ORDER BY id
				LIMIT ${config.batchSize}
			),
			projected_inserts AS (
				INSERT INTO dispatch_jobs_read (
					id, external_id, source, kind, code, subject, event_id, correlation_id,
					target_url, protocol, service_account_id, client_id, subscription_id,
					mode, dispatch_pool_id, message_group, sequence, timeout_seconds,
					status, max_retries, retry_strategy, scheduled_for, expires_at,
					attempt_count, last_attempt_at, completed_at, duration_millis, last_error,
					idempotency_key, is_completed, is_terminal,
					application, subdomain, aggregate,
					created_at, updated_at, projected_at
				)
				SELECT
					b.dispatch_job_id,
					b.payload->>'externalId',
					b.payload->>'source',
					b.payload->>'kind',
					b.payload->>'code',
					b.payload->>'subject',
					b.payload->>'eventId',
					b.payload->>'correlationId',
					b.payload->>'targetUrl',
					b.payload->>'protocol',
					b.payload->>'serviceAccountId',
					b.payload->>'clientId',
					b.payload->>'subscriptionId',
					b.payload->>'mode',
					b.payload->>'dispatchPoolId',
					b.payload->>'messageGroup',
					(b.payload->>'sequence')::int,
					(b.payload->>'timeoutSeconds')::int,
					b.payload->>'status',
					COALESCE((b.payload->>'maxRetries')::int, 3),
					b.payload->>'retryStrategy',
					(b.payload->>'scheduledFor')::timestamptz,
					(b.payload->>'expiresAt')::timestamptz,
					COALESCE((b.payload->>'attemptCount')::int, 0),
					(b.payload->>'lastAttemptAt')::timestamptz,
					(b.payload->>'completedAt')::timestamptz,
					(b.payload->>'durationMillis')::bigint,
					b.payload->>'lastError',
					b.payload->>'idempotencyKey',
					(b.payload->>'isCompleted')::boolean,
					(b.payload->>'isTerminal')::boolean,
					split_part(b.payload->>'code', ':', 1),
					NULLIF(split_part(b.payload->>'code', ':', 2), ''),
					NULLIF(split_part(b.payload->>'code', ':', 3), ''),
					COALESCE((b.payload->>'createdAt')::timestamptz, NOW()),
					COALESCE((b.payload->>'updatedAt')::timestamptz, NOW()),
					NOW()
				FROM batch b
				WHERE b.operation = 'INSERT'
				ON CONFLICT (id) DO UPDATE SET
					status = EXCLUDED.status,
					attempt_count = EXCLUDED.attempt_count,
					last_attempt_at = EXCLUDED.last_attempt_at,
					completed_at = EXCLUDED.completed_at,
					duration_millis = EXCLUDED.duration_millis,
					last_error = EXCLUDED.last_error,
					is_completed = EXCLUDED.is_completed,
					is_terminal = EXCLUDED.is_terminal,
					updated_at = EXCLUDED.updated_at,
					projected_at = NOW()
			),
			projected_updates AS (
				UPDATE dispatch_jobs_read AS t
				SET
					status = COALESCE(src.payload->>'status', t.status),
					attempt_count = COALESCE((src.payload->>'attemptCount')::int, t.attempt_count),
					last_attempt_at = COALESCE((src.payload->>'lastAttemptAt')::timestamptz, t.last_attempt_at),
					completed_at = COALESCE((src.payload->>'completedAt')::timestamptz, t.completed_at),
					duration_millis = COALESCE((src.payload->>'durationMillis')::bigint, t.duration_millis),
					last_error = COALESCE(src.payload->>'lastError', t.last_error),
					is_completed = COALESCE((src.payload->>'isCompleted')::boolean, t.is_completed),
					is_terminal = COALESCE((src.payload->>'isTerminal')::boolean, t.is_terminal),
					updated_at = COALESCE((src.payload->>'updatedAt')::timestamptz, t.updated_at),
					projected_at = NOW()
				FROM (
					SELECT DISTINCT ON (dispatch_job_id) dispatch_job_id, payload
					FROM batch
					WHERE operation = 'UPDATE'
					ORDER BY dispatch_job_id, id DESC
				) src
				WHERE t.id = src.dispatch_job_id
			)
			UPDATE dispatch_job_projection_feed
			SET processed = 1, processed_at = NOW()
			WHERE id IN (SELECT id FROM batch)
		`;

		const count = result.count;
		if (count > 0) {
			logger.debug({ count }, 'Projected dispatch job changes');
		}
		return count;
	}

	async function pollLoop(): Promise<void> {
		while (running) {
			try {
				const processed = await pollAndProject();

				if (processed === 0) {
					await sleep(1000); // No work, sleep 1 second
				} else if (processed < config.batchSize) {
					await sleep(100); // Partial batch, sleep 100ms
				}
				// Full batch: no sleep, immediately poll again
			} catch (err) {
				if (!running) break;
				logger.error({ err }, 'Error in dispatch job projection poll loop');
				await sleep(5000); // Back off on error
			}
		}
	}

	function start(): void {
		if (running) {
			logger.warn('Dispatch job projection service already running');
			return;
		}

		running = true;
		pollLoop().catch((err) => {
			logger.error({ err }, 'Dispatch job projection poll loop exited unexpectedly');
			running = false;
		});
		logger.info({ batchSize: config.batchSize }, 'Dispatch job projection service started');
	}

	function stop(): void {
		if (!running) return;
		logger.info('Stopping dispatch job projection service...');
		running = false;
		logger.info('Dispatch job projection service stopped');
	}

	return { start, stop, isRunning: () => running };
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
