/**
 * Password Reset Token Repository
 *
 * Data access for single-use password reset tokens.
 * Tokens are stored as SHA-256 hashes — never the plaintext URL token.
 */

import { eq } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { TransactionContext } from "@flowcatalyst/persistence";
import { generate } from "@flowcatalyst/tsid";
import { passwordResetTokens } from "../schema/index.js";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

/**
 * Password reset token repository interface.
 */
export interface PasswordResetTokenRepository {
	/** Insert a new token record. */
	insert(record: {
		principalId: string;
		tokenHash: string;
		expiresAt: Date;
	}): Promise<void>;

	/** Find a token record by its SHA-256 hash. Returns undefined if not found. */
	findByTokenHash(
		hash: string,
	): Promise<{ id: string; principalId: string; expiresAt: Date } | undefined>;

	/** Delete all tokens for a principal (used when issuing a new token). */
	deleteByPrincipalId(principalId: string): Promise<void>;

	/** Delete a single token by ID (single-use deletion after successful reset). */
	deleteById(id: string): Promise<void>;
}

/**
 * Create a PasswordResetToken repository backed by Drizzle/Postgres.
 */
export function createPasswordResetTokenRepository(
	defaultDb: AnyDb,
): PasswordResetTokenRepository {
	const db = (tx?: TransactionContext): AnyDb =>
		(tx?.db as AnyDb) ?? defaultDb;

	return {
		async insert({ principalId, tokenHash, expiresAt }) {
			const id = generate("PASSWORD_RESET_TOKEN");
			await db()
				.insert(passwordResetTokens)
				.values({ id, principalId, tokenHash, expiresAt });
		},

		async findByTokenHash(hash: string) {
			const [record] = await db()
				.select()
				.from(passwordResetTokens)
				.where(eq(passwordResetTokens.tokenHash, hash))
				.limit(1);

			if (!record) return undefined;

			return {
				id: record.id,
				principalId: record.principalId,
				expiresAt: record.expiresAt,
			};
		},

		async deleteByPrincipalId(principalId: string) {
			await db()
				.delete(passwordResetTokens)
				.where(eq(passwordResetTokens.principalId, principalId));
		},

		async deleteById(id: string) {
			await db()
				.delete(passwordResetTokens)
				.where(eq(passwordResetTokens.id, id));
		},
	};
}
