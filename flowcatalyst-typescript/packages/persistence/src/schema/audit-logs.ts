/**
 * Audit Logs Schema
 *
 * Database schema for audit log records. Every state change in the system
 * creates an audit log entry linking the entity, operation, and principal.
 */

import { pgTable, varchar, jsonb, index } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from './common.js';

/**
 * Audit logs table schema.
 */
export const auditLogs = pgTable(
	'audit_logs',
	{
		// Primary key
		id: tsidColumn('id').primaryKey(),

		// Entity identification
		entityType: varchar('entity_type', { length: 100 }).notNull(),
		entityId: tsidColumn('entity_id').notNull(),

		// Operation details
		operation: varchar('operation', { length: 100 }).notNull(),
		operationJson: jsonb('operation_json'),

		// Who performed the operation
		principalId: tsidColumn('principal_id'),

		// When the operation was performed
		performedAt: timestampColumn('performed_at').notNull().defaultNow(),
	},
	(table) => [
		// Index for entity history queries
		index('idx_audit_logs_entity').on(table.entityType, table.entityId),
		// Index for chronological queries
		index('idx_audit_logs_performed').on(table.performedAt),
		// Index for principal queries
		index('idx_audit_logs_principal').on(table.principalId),
		// Index for operation type queries
		index('idx_audit_logs_operation').on(table.operation),
	],
);

/**
 * Audit log entity type (select result).
 */
export type AuditLogRecord = typeof auditLogs.$inferSelect;

/**
 * New audit log type (insert input).
 */
export type NewAuditLog = typeof auditLogs.$inferInsert;
