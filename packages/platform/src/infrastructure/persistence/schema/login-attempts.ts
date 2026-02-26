/**
 * Login Attempts Schema
 *
 * Database schema for recording authentication attempt events.
 * Both user password logins and service account OAuth token requests
 * are captured here for security auditing and operational visibility.
 */

import { pgTable, varchar, text, index } from "drizzle-orm/pg-core";
import { tsidColumn, timestampColumn } from "@flowcatalyst/persistence";

/**
 * Login attempts table schema.
 */
export const loginAttempts = pgTable(
	"iam_login_attempts",
	{
		// Primary key
		id: tsidColumn("id").primaryKey(),

		// Type of attempt
		attemptType: varchar("attempt_type", { length: 20 }).notNull(),

		// Outcome
		outcome: varchar("outcome", { length: 20 }).notNull(),

		// Reason for failure (null on success)
		failureReason: varchar("failure_reason", { length: 100 }),

		// Identifier used in the attempt (email for user login, client_id for service account)
		identifier: varchar("identifier", { length: 255 }),

		// Principal ID if known (on success, or if principal was found but auth failed)
		principalId: varchar("principal_id", { length: 17 }),

		// Request metadata
		ipAddress: varchar("ip_address", { length: 45 }),
		userAgent: text("user_agent"),

		// When the attempt occurred
		attemptedAt: timestampColumn("attempted_at").notNull(),
	},
	(table) => [
		index("idx_iam_login_attempts_attempted_at").on(table.attemptedAt),
		index("idx_iam_login_attempts_outcome").on(table.outcome),
		index("idx_iam_login_attempts_identifier").on(table.identifier),
		index("idx_iam_login_attempts_principal_id").on(table.principalId),
	],
);

/**
 * Login attempt entity type (select result).
 */
export type LoginAttemptRecord = typeof loginAttempts.$inferSelect;

/**
 * New login attempt type (insert input).
 */
export type NewLoginAttemptRecord = typeof loginAttempts.$inferInsert;
