/**
 * Event Projection Service
 *
 * Projects events from msg_event_projection_feed to msg_events_read using pure SQL.
 *
 * Uses a single writable CTE per poll cycle: one atomic statement that
 * selects the batch, inserts into msg_events_read, and marks feed entries
 * as processed. Zero application-layer data transfer - everything stays
 * in PostgreSQL.
 *
 * Algorithm:
 *   1. Single CTE: batch SELECT -> INSERT msg_events_read -> UPDATE feed
 *   2. Sleep: 0ms if full batch, 100ms if partial, 1000ms if zero results
 *
 * Type hierarchy parsing:
 *   type: "orders:fulfillment:shipment:shipped"
 *   -> application: "orders", subdomain: "fulfillment", aggregate: "shipment"
 */

import type postgres from 'postgres';
import type { Logger } from '@flowcatalyst/logging';

export interface EventProjectionConfig {
  readonly enabled: boolean;
  readonly batchSize: number;
}

export interface EventProjectionService {
  start(): void;
  stop(): void;
  isRunning(): boolean;
}

export function createEventProjectionService(
  sql: postgres.Sql,
  config: EventProjectionConfig,
  logger: Logger,
): EventProjectionService {
  let running = false;

  /**
   * Single-statement batch projection using writable CTE.
   *
   * 1. `batch` CTE: selects unprocessed feed entries (LIMIT batchSize)
   * 2. `projected` CTE: UPSERTs into msg_events_read from batch payloads
   * 3. Main UPDATE: marks batch entries as processed
   *
   * All three operations execute atomically in one round-trip.
   * Data never leaves PostgreSQL - JSONB extraction happens in-engine.
   */
  async function pollAndProject(): Promise<number> {
    const result = await sql`
			WITH batch AS (
				SELECT id, event_id, payload
				FROM msg_event_projection_feed
				WHERE processed = 0
				ORDER BY id
				LIMIT ${config.batchSize}
			),
			projected AS (
				INSERT INTO msg_events_read (
					id, spec_version, type, source, subject, time, data,
					correlation_id, causation_id, deduplication_id, message_group,
					client_id, application, subdomain, aggregate, projected_at
				)
				SELECT
					b.event_id,
					b.payload->>'specVersion',
					b.payload->>'type',
					b.payload->>'source',
					b.payload->>'subject',
					(b.payload->>'time')::timestamptz,
					b.payload->>'data',
					b.payload->>'correlationId',
					b.payload->>'causationId',
					b.payload->>'deduplicationId',
					b.payload->>'messageGroup',
					b.payload->>'clientId',
					split_part(b.payload->>'type', ':', 1),
					NULLIF(split_part(b.payload->>'type', ':', 2), ''),
					NULLIF(split_part(b.payload->>'type', ':', 3), ''),
					NOW()
				FROM batch b
				ON CONFLICT (id) DO NOTHING
			)
			UPDATE msg_event_projection_feed
			SET processed = 1, processed_at = NOW()
			WHERE id IN (SELECT id FROM batch)
		`;

    const count = result.count;
    if (count > 0) {
      logger.debug({ count }, 'Projected events');
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
        logger.error({ err }, 'Error in event projection poll loop');
        await sleep(5000); // Back off on error
      }
    }
  }

  function start(): void {
    if (running) {
      logger.warn('Event projection service already running');
      return;
    }

    running = true;
    pollLoop().catch((err) => {
      logger.error({ err }, 'Event projection poll loop exited unexpectedly');
      running = false;
    });
    logger.info({ batchSize: config.batchSize }, 'Event projection service started');
  }

  function stop(): void {
    if (!running) return;
    logger.info('Stopping event projection service...');
    running = false;
    logger.info('Event projection service stopped');
  }

  return { start, stop, isRunning: () => running };
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
