/**
 * Outbox Processor Model
 *
 * Types and status codes for outbox message processing.
 * Status codes match the SDK's OutboxStatus for interoperability.
 */

/**
 * Outbox item type — determines which batch API endpoint to use.
 */
export type OutboxItemType = 'EVENT' | 'DISPATCH_JOB' | 'AUDIT_LOG';

/**
 * Outbox status codes (matches SDK OutboxStatus).
 *   0 = PENDING
 *   1 = SUCCESS
 *   2 = BAD_REQUEST (permanent failure — 400)
 *   3 = INTERNAL_ERROR (retriable — 500)
 *   4 = UNAUTHORIZED (permanent — 401)
 *   5 = FORBIDDEN (permanent — 403)
 *   6 = GATEWAY_ERROR (retriable — 502/503/504)
 *   9 = IN_PROGRESS (crash recovery marker)
 */
export type OutboxStatus = 0 | 1 | 2 | 3 | 4 | 5 | 6 | 9;

export const OutboxStatus = {
  PENDING: 0 as OutboxStatus,
  SUCCESS: 1 as OutboxStatus,
  BAD_REQUEST: 2 as OutboxStatus,
  INTERNAL_ERROR: 3 as OutboxStatus,
  UNAUTHORIZED: 4 as OutboxStatus,
  FORBIDDEN: 5 as OutboxStatus,
  GATEWAY_ERROR: 6 as OutboxStatus,
  IN_PROGRESS: 9 as OutboxStatus,
} as const;

/**
 * Outbox item from the outbox_messages table.
 */
export interface OutboxItem {
  readonly id: number;
  readonly type: OutboxItemType;
  readonly status: OutboxStatus;
  readonly payload: string;
  readonly messageGroup: string;
  readonly retryCount: number;
  readonly maxRetries: number;
  readonly errorMessage: string | null;
  readonly createdAt: Date;
  readonly updatedAt: Date;
}

/**
 * Check if a status code is retriable (server/gateway errors).
 */
export function isRetriableStatus(status: OutboxStatus): boolean {
  return status === OutboxStatus.INTERNAL_ERROR || status === OutboxStatus.GATEWAY_ERROR;
}

/**
 * Check if a status code is a permanent failure (no retry).
 */
export function isPermanentFailure(status: OutboxStatus): boolean {
  return (
    status === OutboxStatus.BAD_REQUEST ||
    status === OutboxStatus.UNAUTHORIZED ||
    status === OutboxStatus.FORBIDDEN
  );
}

/**
 * Map an HTTP status code to an OutboxStatus.
 */
export function httpStatusToOutboxStatus(httpStatus: number): OutboxStatus {
  if (httpStatus >= 200 && httpStatus < 300) return OutboxStatus.SUCCESS;
  if (httpStatus === 400) return OutboxStatus.BAD_REQUEST;
  if (httpStatus === 401) return OutboxStatus.UNAUTHORIZED;
  if (httpStatus === 403) return OutboxStatus.FORBIDDEN;
  if (httpStatus === 502 || httpStatus === 503 || httpStatus === 504)
    return OutboxStatus.GATEWAY_ERROR;
  if (httpStatus >= 500) return OutboxStatus.INTERNAL_ERROR;
  // Any other 4xx → BAD_REQUEST
  return OutboxStatus.BAD_REQUEST;
}
