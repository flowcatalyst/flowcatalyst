/**
 * PostgreSQL Outbox Repository
 *
 * Uses postgres.js (raw SQL) to access the customer's outbox_messages table.
 * This is NOT the platform database — it connects to a separate customer DB.
 * Uses simple SELECT/UPDATE with status codes, no row locking.
 */

import postgres from "postgres";
import {
	OutboxStatus,
	type OutboxItem,
	type OutboxItemType,
} from "../model.js";
import type { OutboxRepository } from "./outbox-repository.js";
import type { OutboxProcessorConfig } from "../env.js";

export function createPostgresOutboxRepository(
	config: OutboxProcessorConfig,
): OutboxRepository {
	const sql = postgres(config.databaseUrl, {
		max: 10,
		idle_timeout: 30,
	});

	function getTableName(type: OutboxItemType): string {
		switch (type) {
			case "EVENT":
				return config.eventsTable;
			case "DISPATCH_JOB":
				return config.dispatchJobsTable;
			case "AUDIT_LOG":
				return config.auditLogsTable;
		}
	}

	function mapRow(
		row: Record<string, unknown>,
		type: OutboxItemType,
	): OutboxItem {
		return {
			id: row["id"] as number,
			type,
			messageGroup: (row["message_group"] as string) ?? "default",
			payload: row["payload"] as string,
			status: row["status"] as number as OutboxStatus,
			retryCount: (row["retry_count"] as number) ?? 0,
			maxRetries: config.maxRetries,
			errorMessage: (row["error_message"] as string) ?? null,
			createdAt: row["created_at"] as Date,
			updatedAt: row["updated_at"] as Date,
		};
	}

	return {
		getTableName,

		async fetchPending(type, limit) {
			const table = getTableName(type);
			const rows = await sql.unsafe(
				`SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
         FROM ${table}
         WHERE status = ${OutboxStatus.PENDING} AND type = $1
         ORDER BY message_group, created_at
         LIMIT $2`,
				[type, limit],
			);
			return rows.map((r) => mapRow(r, type));
		},

		async markAsInProgress(type, ids) {
			if (ids.length === 0) return;
			const table = getTableName(type);
			await sql.unsafe(
				`UPDATE ${table}
         SET status = ${OutboxStatus.IN_PROGRESS}, updated_at = NOW()
         WHERE id = ANY($1)`,
				[ids],
			);
		},

		async markWithStatus(type, ids, status) {
			if (ids.length === 0) return;
			const table = getTableName(type);
			await sql.unsafe(
				`UPDATE ${table}
         SET status = $1, updated_at = NOW()
         WHERE id = ANY($2)`,
				[status, ids],
			);
		},

		async markWithStatusAndError(type, ids, status, errorMessage) {
			if (ids.length === 0) return;
			const table = getTableName(type);
			await sql.unsafe(
				`UPDATE ${table}
         SET status = $1, error_message = $2, updated_at = NOW()
         WHERE id = ANY($3)`,
				[status, errorMessage, ids],
			);
		},

		async incrementRetryCount(type, ids) {
			if (ids.length === 0) return;
			const table = getTableName(type);
			await sql.unsafe(
				`UPDATE ${table}
         SET status = ${OutboxStatus.PENDING}, retry_count = retry_count + 1, updated_at = NOW()
         WHERE id = ANY($1)`,
				[ids],
			);
		},

		async fetchStuckItems(type) {
			const table = getTableName(type);
			const rows = await sql.unsafe(
				`SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
         FROM ${table}
         WHERE status = ${OutboxStatus.IN_PROGRESS} AND type = $1
         ORDER BY created_at`,
				[type],
			);
			return rows.map((r) => mapRow(r, type));
		},

		async resetStuckItems(type, ids) {
			if (ids.length === 0) return;
			const table = getTableName(type);
			await sql.unsafe(
				`UPDATE ${table}
         SET status = ${OutboxStatus.PENDING}, updated_at = NOW()
         WHERE id = ANY($1)`,
				[ids],
			);
		},

		async fetchRecoverableItems(type, timeoutSeconds, limit) {
			const table = getTableName(type);
			const rows = await sql.unsafe(
				`SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
         FROM ${table}
         WHERE status IN (${OutboxStatus.IN_PROGRESS}, ${OutboxStatus.BAD_REQUEST}, ${OutboxStatus.INTERNAL_ERROR}, ${OutboxStatus.UNAUTHORIZED}, ${OutboxStatus.FORBIDDEN}, ${OutboxStatus.GATEWAY_ERROR})
           AND type = $1
           AND updated_at < NOW() - INTERVAL '1 second' * $2
         ORDER BY created_at
         LIMIT $3`,
				[type, timeoutSeconds, limit],
			);
			return rows.map((r) => mapRow(r, type));
		},

		async resetRecoverableItems(type, ids) {
			if (ids.length === 0) return;
			const table = getTableName(type);
			await sql.unsafe(
				`UPDATE ${table}
         SET status = ${OutboxStatus.PENDING}, updated_at = NOW()
         WHERE id = ANY($1)`,
				[ids],
			);
		},

		async close() {
			await sql.end();
		},
	};
}
