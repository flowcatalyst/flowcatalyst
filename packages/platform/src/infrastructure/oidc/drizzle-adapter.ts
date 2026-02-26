/**
 * Drizzle Adapter for oidc-provider
 *
 * Implements the oidc-provider Adapter interface using Drizzle ORM with PostgreSQL.
 * This adapter handles storage for all OIDC artifacts (tokens, sessions, codes, etc.)
 */

import type { Adapter, AdapterPayload } from "oidc-provider";
import { eq, and, sql, lt } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import { oidcPayloads } from "../persistence/schema/index.js";

/**
 * Optional dynamic client loader.
 * When the Client adapter model doesn't find a client in storage,
 * it falls back to this function to load from the OAuth client repository.
 */
export type ClientLoader = (
	clientId: string,
) => Promise<Record<string, unknown> | undefined>;

/**
 * Creates a Drizzle adapter factory for oidc-provider.
 *
 * @param db - Drizzle database instance
 * @param clientLoader - Optional function to dynamically load OAuth clients from the repository
 * @returns Adapter class constructor
 */
export function createDrizzleAdapterFactory(
	db: PostgresJsDatabase,
	clientLoader?: ClientLoader,
): new (
	name: string,
) => Adapter {
	return class DrizzleAdapter implements Adapter {
		readonly _name: string;

		constructor(name: string) {
			this._name = name;
		}

		/**
		 * Builds the composite key for storage.
		 * Format: {type}:{id} to ensure uniqueness across model types.
		 */
		_key(id: string): string {
			return `${this._name}:${id}`;
		}

		/**
		 * Upsert a payload into storage.
		 */
		async upsert(
			id: string,
			payload: AdapterPayload,
			expiresIn: number,
		): Promise<void> {
			const key = this._key(id);
			const expiresAt = expiresIn
				? new Date(Date.now() + expiresIn * 1000)
				: null;

			// Extract special fields that oidc-provider uses for lookups
			const grantId = payload.grantId ?? null;
			const userCode = payload.userCode ?? null;
			const uid = payload.uid ?? null;

			await db
				.insert(oidcPayloads)
				.values({
					id: key,
					type: this._name,
					payload: payload as Record<string, unknown>,
					grantId,
					userCode,
					uid,
					expiresAt,
				})
				.onConflictDoUpdate({
					target: oidcPayloads.id,
					set: {
						payload: payload as Record<string, unknown>,
						grantId,
						userCode,
						uid,
						expiresAt,
					},
				});
		}

		/**
		 * Find a payload by ID.
		 * For the Client model, falls back to dynamic loading from the OAuth client repository.
		 */
		async find(id: string): Promise<AdapterPayload | undefined> {
			const key = this._key(id);

			const [record] = await db
				.select()
				.from(oidcPayloads)
				.where(eq(oidcPayloads.id, key))
				.limit(1);

			if (record) {
				// Check if expired — for Client model, delete stale cache and fall through
				// to the dynamic client loader below instead of returning undefined.
				if (record.expiresAt && record.expiresAt < new Date()) {
					if (this._name === "Client") {
						await this.destroy(id);
					} else {
						return undefined;
					}
				} else {
					return this._hydratePayload(record);
				}
			}

			// For Client model, fall back to dynamic loading from OAuth client repository
			if (this._name === "Client" && clientLoader) {
				const metadata = await clientLoader(id);
				if (metadata) {
					// Cache in storage for future lookups (24 hours)
					await this.upsert(id, metadata as AdapterPayload, 24 * 3600);
					return metadata as AdapterPayload;
				}
			}

			return undefined;
		}

		/**
		 * Find a payload by user code (device authorization flow).
		 */
		async findByUserCode(
			userCode: string,
		): Promise<AdapterPayload | undefined> {
			const [record] = await db
				.select()
				.from(oidcPayloads)
				.where(
					and(
						eq(oidcPayloads.type, this._name),
						eq(oidcPayloads.userCode, userCode),
					),
				)
				.limit(1);

			if (!record) {
				return undefined;
			}

			if (record.expiresAt && record.expiresAt < new Date()) {
				return undefined;
			}

			return this._hydratePayload(record);
		}

		/**
		 * Find a payload by UID.
		 */
		async findByUid(uid: string): Promise<AdapterPayload | undefined> {
			const [record] = await db
				.select()
				.from(oidcPayloads)
				.where(
					and(eq(oidcPayloads.type, this._name), eq(oidcPayloads.uid, uid)),
				)
				.limit(1);

			if (!record) {
				return undefined;
			}

			if (record.expiresAt && record.expiresAt < new Date()) {
				return undefined;
			}

			return this._hydratePayload(record);
		}

		/**
		 * Mark a payload as consumed (single-use enforcement).
		 */
		async consume(id: string): Promise<void> {
			const key = this._key(id);

			await db
				.update(oidcPayloads)
				.set({ consumedAt: new Date() })
				.where(eq(oidcPayloads.id, key));
		}

		/**
		 * Destroy (delete) a payload by ID.
		 */
		async destroy(id: string): Promise<void> {
			const key = this._key(id);
			await db.delete(oidcPayloads).where(eq(oidcPayloads.id, key));
		}

		/**
		 * Revoke all payloads associated with a grant ID.
		 * This is called when a user revokes consent or when tokens are rotated.
		 */
		async revokeByGrantId(grantId: string): Promise<void> {
			await db.delete(oidcPayloads).where(eq(oidcPayloads.grantId, grantId));
		}

		/**
		 * Hydrate a database record into an AdapterPayload.
		 */
		_hydratePayload(record: typeof oidcPayloads.$inferSelect): AdapterPayload {
			const payload = record.payload as AdapterPayload;

			// Add consumed timestamp if present
			if (record.consumedAt) {
				return {
					...payload,
					consumed: Math.floor(record.consumedAt.getTime() / 1000),
				};
			}

			return payload;
		}
	};
}

/**
 * Cleanup expired payloads.
 * Should be called periodically (e.g., via cron job or scheduled task).
 */
export async function cleanupExpiredPayloads(
	db: PostgresJsDatabase,
): Promise<number> {
	const result = await db
		.delete(oidcPayloads)
		.where(lt(oidcPayloads.expiresAt, new Date()))
		.returning({ id: oidcPayloads.id });

	return result.length;
}

/**
 * Invalidate the cached OIDC client metadata for a given OAuth client_id.
 * Call this after rotating/regenerating a client secret so oidc-provider
 * reloads fresh metadata (with the new secret) on the next token request.
 */
export async function invalidateOidcClientCache(
	db: PostgresJsDatabase,
	clientId: string,
): Promise<void> {
	const key = `Client:${clientId}`;
	await db.delete(oidcPayloads).where(eq(oidcPayloads.id, key));
}

/**
 * Get payload statistics for monitoring.
 */
export async function getPayloadStats(
	db: PostgresJsDatabase,
): Promise<Array<{ type: string; count: number }>> {
	const result = await db
		.select({
			type: oidcPayloads.type,
			count: sql<number>`count(*)::int`,
		})
		.from(oidcPayloads)
		.groupBy(oidcPayloads.type);

	return result;
}
