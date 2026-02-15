/**
 * FlowCatalyst API Client
 *
 * HTTP client for the platform's batch API endpoints.
 * Sends batches of outbox items and maps HTTP responses to OutboxStatus.
 */

import { OutboxStatus, httpStatusToOutboxStatus, type OutboxItem, type OutboxItemType } from './model.js';
import type { OutboxProcessorConfig } from './env.js';
import type { Logger } from 'pino';

/**
 * Result of a batch API call.
 */
export interface BatchResult {
  /** Whether all items succeeded. */
  readonly allSuccess: boolean;
  /** Map of failed item IDs to their OutboxStatus. */
  readonly failedItems: Map<string, OutboxStatus>;
  /** Error message if the entire batch failed. */
  readonly errorMessage: string | null;
}

function allSuccess(): BatchResult {
  return { allSuccess: true, failedItems: new Map(), errorMessage: null };
}

function allFailed(ids: string[], status: OutboxStatus, errorMessage: string): BatchResult {
  const failedItems = new Map<string, OutboxStatus>();
  for (const id of ids) {
    failedItems.set(id, status);
  }
  return { allSuccess: false, failedItems, errorMessage };
}

export interface ApiClient {
  createEventsBatch(items: OutboxItem[]): Promise<BatchResult>;
  createDispatchJobsBatch(items: OutboxItem[]): Promise<BatchResult>;
  createAuditLogsBatch(items: OutboxItem[]): Promise<BatchResult>;
}

export function createApiClient(config: OutboxProcessorConfig, logger: Logger): ApiClient {
  const { apiBaseUrl, apiToken } = config;

  async function post(
    path: string,
    items: OutboxItem[],
  ): Promise<BatchResult> {
    const ids = items.map((item) => String(item.id));
    const payloads = items.map((item) => {
      try {
        return JSON.parse(item.payload);
      } catch {
        throw new Error(`Invalid JSON payload for item ${item.id}`);
      }
    });

    const url = `${apiBaseUrl}${path}`;
    const body = JSON.stringify({ items: payloads });

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (apiToken) {
      headers['Authorization'] = `Bearer ${apiToken}`;
    }

    try {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 30_000);

      const response = await fetch(url, {
        method: 'POST',
        headers,
        body,
        signal: controller.signal,
      });

      clearTimeout(timeout);

      const statusCode = response.status;

      if (statusCode >= 200 && statusCode < 300) {
        logger.debug({ path, statusCode }, 'Batch API call succeeded');
        return allSuccess();
      }

      const responseBody = await response.text().catch(() => '');
      const outboxStatus = httpStatusToOutboxStatus(statusCode);
      const errorMessage = `API error: ${statusCode} - ${responseBody}`;
      logger.error({ path, statusCode, responseBody }, 'Batch API call failed');

      return allFailed(ids, outboxStatus, errorMessage);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);

      if (error instanceof Error && error.name === 'AbortError') {
        logger.error({ path }, 'Batch API call timed out');
        return allFailed(ids, OutboxStatus.GATEWAY_ERROR, `Request timeout: ${message}`);
      }

      logger.error({ path, error: message }, 'Batch API call failed with network error');
      return allFailed(ids, OutboxStatus.GATEWAY_ERROR, `IO error: ${message}`);
    }
  }

  return {
    createEventsBatch(items) {
      return post('/api/events/batch', items);
    },
    createDispatchJobsBatch(items) {
      return post('/api/dispatch/jobs/batch', items);
    },
    createAuditLogsBatch(items) {
      return post('/api/audit-logs/batch', items);
    },
  };
}
