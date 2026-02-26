/**
 * Outbox Repository Interface
 *
 * Abstracts access to the customer's outbox_messages table.
 */

import type { OutboxItem, OutboxItemType, OutboxStatus } from "../model.js";

export interface OutboxRepository {
	/** Fetch pending items (status=0) of a given type, ordered by message_group, created_at. */
	fetchPending(type: OutboxItemType, limit: number): Promise<OutboxItem[]>;

	/** Mark items as in-progress (status=9). */
	markAsInProgress(type: OutboxItemType, ids: string[]): Promise<void>;

	/** Mark items with a specific status. */
	markWithStatus(
		type: OutboxItemType,
		ids: string[],
		status: OutboxStatus,
	): Promise<void>;

	/** Mark items with a status and error message. */
	markWithStatusAndError(
		type: OutboxItemType,
		ids: string[],
		status: OutboxStatus,
		errorMessage: string,
	): Promise<void>;

	/** Increment retry count and reset to PENDING. */
	incrementRetryCount(type: OutboxItemType, ids: string[]): Promise<void>;

	/** Fetch items stuck in IN_PROGRESS (status=9). */
	fetchStuckItems(type: OutboxItemType): Promise<OutboxItem[]>;

	/** Reset stuck items back to PENDING (status=0). */
	resetStuckItems(type: OutboxItemType, ids: string[]): Promise<void>;

	/** Fetch items in error states older than timeoutSeconds. */
	fetchRecoverableItems(
		type: OutboxItemType,
		timeoutSeconds: number,
		limit: number,
	): Promise<OutboxItem[]>;

	/** Reset recoverable items back to PENDING. */
	resetRecoverableItems(type: OutboxItemType, ids: string[]): Promise<void>;

	/** Get the table name for a given type. */
	getTableName(type: OutboxItemType): string;

	/** Close the database connection. */
	close(): Promise<void>;
}
