/**
 * Password Reset Tokens Schema
 *
 * Single-use tokens for internal user password resets.
 * Tokens are hashed (SHA-256) before storage to prevent DB breach exposing valid tokens.
 */

import { pgTable, varchar, timestamp, uniqueIndex, index } from "drizzle-orm/pg-core";
import { tsidColumn } from "@flowcatalyst/persistence";

/**
 * Password reset tokens table.
 */
export const passwordResetTokens = pgTable(
	"iam_password_reset_tokens",
	{
		// Primary key (TSID with "prt" prefix)
		id: tsidColumn("id").primaryKey(),

		// FK to iam_principals (not enforced to avoid cascading issues on delete)
		principalId: varchar("principal_id", { length: 17 }).notNull(),

		// SHA-256 hex of the plaintext URL token (64 chars)
		tokenHash: varchar("token_hash", { length: 64 }).notNull(),

		// Token expiry (15 minutes after creation)
		expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),

		// Creation timestamp
		createdAt: timestamp("created_at", { withTimezone: true })
			.notNull()
			.defaultNow(),
	},
	(table) => [
		uniqueIndex("idx_prt_token_hash").on(table.tokenHash),
		index("idx_prt_principal_id").on(table.principalId),
	],
);

/**
 * Password reset token entity type (select result).
 */
export type PasswordResetTokenRecord = typeof passwordResetTokens.$inferSelect;

/**
 * New password reset token type (insert input).
 */
export type NewPasswordResetTokenRecord = typeof passwordResetTokens.$inferInsert;
