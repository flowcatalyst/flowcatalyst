/**
 * EventType Repository
 *
 * Data access for EventType entities with spec version sub-queries.
 */

import { eq, sql, and, like, inArray } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { PaginatedRepository, PagedResult, TransactionContext } from '@flowcatalyst/persistence';
import { createPagedResult } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	eventTypes,
	eventTypeSpecVersions,
	type EventTypeRecord,
	type NewEventTypeRecord,
	type EventTypeSpecVersionRecord,
	type NewEventTypeSpecVersionRecord,
} from '../schema/index.js';
import type {
	EventType,
	NewEventType,
	SpecVersion,
	NewSpecVersion,
	EventTypeStatus,
	EventTypeSource,
	SchemaType,
	SpecVersionStatus,
} from '../../../domain/index.js';

/**
 * Filters for event type listing.
 */
export interface EventTypeFilters {
	readonly status?: EventTypeStatus | undefined;
	readonly applications?: string[] | undefined;
	readonly subdomains?: string[] | undefined;
	readonly aggregates?: string[] | undefined;
}

/**
 * EventType repository interface.
 */
export interface EventTypeRepository extends PaginatedRepository<EventType> {
	findByCode(code: string, tx?: TransactionContext): Promise<EventType | undefined>;
	existsByCode(code: string, tx?: TransactionContext): Promise<boolean>;
	findByCodePrefix(prefix: string, tx?: TransactionContext): Promise<EventType[]>;
	findWithFilters(filters: EventTypeFilters, tx?: TransactionContext): Promise<EventType[]>;
	findDistinctApplications(tx?: TransactionContext): Promise<string[]>;
	findDistinctSubdomains(applications?: string[], tx?: TransactionContext): Promise<string[]>;
	findDistinctAggregates(
		applications?: string[],
		subdomains?: string[],
		tx?: TransactionContext,
	): Promise<string[]>;
	// Spec version operations
	insertSpecVersion(specVersion: NewSpecVersion, tx?: TransactionContext): Promise<SpecVersion>;
	updateSpecVersion(specVersion: SpecVersion, tx?: TransactionContext): Promise<SpecVersion>;
	findSpecVersionsByEventTypeId(eventTypeId: string, tx?: TransactionContext): Promise<SpecVersion[]>;
}

/**
 * Create an EventType repository.
 */
export function createEventTypeRepository(defaultDb: AnyDb): EventTypeRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	/**
	 * Load spec versions for an event type.
	 */
	async function loadSpecVersions(eventTypeId: string, txCtx?: TransactionContext): Promise<SpecVersion[]> {
		const records = await db(txCtx)
			.select()
			.from(eventTypeSpecVersions)
			.where(eq(eventTypeSpecVersions.eventTypeId, eventTypeId))
			.orderBy(eventTypeSpecVersions.version);

		return records.map(recordToSpecVersion);
	}

	/**
	 * Convert a record to a full EventType with spec versions.
	 */
	async function hydrate(record: EventTypeRecord, txCtx?: TransactionContext): Promise<EventType> {
		const specVersions = await loadSpecVersions(record.id, txCtx);
		return recordToEventType(record, specVersions);
	}

	return {
		async findById(id: string, tx?: TransactionContext): Promise<EventType | undefined> {
			const [record] = await db(tx)
				.select()
				.from(eventTypes)
				.where(eq(eventTypes.id, id))
				.limit(1);

			if (!record) return undefined;
			return hydrate(record, tx);
		},

		async findByCode(code: string, tx?: TransactionContext): Promise<EventType | undefined> {
			const [record] = await db(tx)
				.select()
				.from(eventTypes)
				.where(eq(eventTypes.code, code))
				.limit(1);

			if (!record) return undefined;
			return hydrate(record, tx);
		},

		async findAll(tx?: TransactionContext): Promise<EventType[]> {
			const records = await db(tx)
				.select()
				.from(eventTypes)
				.orderBy(eventTypes.code);
			return Promise.all(records.map((r) => hydrate(r, tx)));
		},

		async findPaged(page: number, pageSize: number, tx?: TransactionContext): Promise<PagedResult<EventType>> {
			const [countResult] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(eventTypes);
			const totalItems = Number(countResult?.count ?? 0);

			const records = await db(tx)
				.select()
				.from(eventTypes)
				.limit(pageSize)
				.offset(page * pageSize)
				.orderBy(eventTypes.code);

			const items = await Promise.all(records.map((r) => hydrate(r, tx)));
			return createPagedResult(items, page, pageSize, totalItems);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(eventTypes);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(eventTypes)
				.where(eq(eventTypes.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByCode(code: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(eventTypes)
				.where(eq(eventTypes.code, code));
			return Number(result?.count ?? 0) > 0;
		},

		async findByCodePrefix(prefix: string, tx?: TransactionContext): Promise<EventType[]> {
			const records = await db(tx)
				.select()
				.from(eventTypes)
				.where(like(eventTypes.code, `${prefix}%`))
				.orderBy(eventTypes.code);
			return Promise.all(records.map((r) => hydrate(r, tx)));
		},

		async findWithFilters(filters: EventTypeFilters, tx?: TransactionContext): Promise<EventType[]> {
			const conditions = [];

			if (filters.status) {
				conditions.push(eq(eventTypes.status, filters.status));
			}
			if (filters.applications && filters.applications.length > 0) {
				conditions.push(inArray(eventTypes.application, filters.applications));
			}
			if (filters.subdomains && filters.subdomains.length > 0) {
				conditions.push(inArray(eventTypes.subdomain, filters.subdomains));
			}
			if (filters.aggregates && filters.aggregates.length > 0) {
				conditions.push(inArray(eventTypes.aggregate, filters.aggregates));
			}

			if (conditions.length === 0) {
				return this.findAll(tx);
			}

			const records = await db(tx)
				.select()
				.from(eventTypes)
				.where(conditions.length === 1 ? conditions[0]! : and(...conditions))
				.orderBy(eventTypes.code);
			return Promise.all(records.map((r) => hydrate(r, tx)));
		},

		async findDistinctApplications(tx?: TransactionContext): Promise<string[]> {
			const result = await db(tx)
				.selectDistinct({ value: eventTypes.application })
				.from(eventTypes)
				.orderBy(eventTypes.application);
			return result.map((r) => r.value);
		},

		async findDistinctSubdomains(applications?: string[], tx?: TransactionContext): Promise<string[]> {
			const query = db(tx)
				.selectDistinct({ value: eventTypes.subdomain })
				.from(eventTypes);

			const result = applications && applications.length > 0
				? await query.where(inArray(eventTypes.application, applications)).orderBy(eventTypes.subdomain)
				: await query.orderBy(eventTypes.subdomain);

			return result.map((r) => r.value);
		},

		async findDistinctAggregates(
			applications?: string[],
			subdomains?: string[],
			tx?: TransactionContext,
		): Promise<string[]> {
			const conditions = [];

			if (applications && applications.length > 0) {
				conditions.push(inArray(eventTypes.application, applications));
			}
			if (subdomains && subdomains.length > 0) {
				conditions.push(inArray(eventTypes.subdomain, subdomains));
			}

			const query = db(tx)
				.selectDistinct({ value: eventTypes.aggregate })
				.from(eventTypes);

			const result = conditions.length > 0
				? await query.where(conditions.length === 1 ? conditions[0]! : and(...conditions)).orderBy(eventTypes.aggregate)
				: await query.orderBy(eventTypes.aggregate);

			return result.map((r) => r.value);
		},

		async insert(entity: NewEventType, tx?: TransactionContext): Promise<EventType> {
			const now = new Date();
			const record: NewEventTypeRecord = {
				id: entity.id,
				code: entity.code,
				name: entity.name,
				description: entity.description,
				status: entity.status,
				source: entity.source,
				clientScoped: entity.clientScoped,
				application: entity.application,
				subdomain: entity.subdomain,
				aggregate: entity.aggregate,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(eventTypes).values(record);

			// Insert spec versions if any
			for (const sv of entity.specVersions) {
				await this.insertSpecVersion(sv, tx);
			}

			return this.findById(entity.id, tx) as Promise<EventType>;
		},

		async update(entity: EventType, tx?: TransactionContext): Promise<EventType> {
			const now = new Date();

			await db(tx)
				.update(eventTypes)
				.set({
					name: entity.name,
					description: entity.description,
					status: entity.status,
					updatedAt: now,
				})
				.where(eq(eventTypes.id, entity.id));

			// Update spec versions
			for (const sv of entity.specVersions) {
				const [existing] = await db(tx)
					.select({ count: sql<number>`count(*)` })
					.from(eventTypeSpecVersions)
					.where(eq(eventTypeSpecVersions.id, sv.id));

				if (Number(existing?.count ?? 0) > 0) {
					await this.updateSpecVersion(sv, tx);
				} else {
					await this.insertSpecVersion(sv, tx);
				}
			}

			return this.findById(entity.id, tx) as Promise<EventType>;
		},

		async persist(entity: NewEventType, tx?: TransactionContext): Promise<EventType> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as EventType, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			// Delete spec versions first
			await db(tx).delete(eventTypeSpecVersions).where(eq(eventTypeSpecVersions.eventTypeId, id));
			await db(tx).delete(eventTypes).where(eq(eventTypes.id, id));
			return true;
		},

		async delete(entity: EventType, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},

		// Spec version operations

		async insertSpecVersion(specVersion: NewSpecVersion, tx?: TransactionContext): Promise<SpecVersion> {
			const now = new Date();
			const record: NewEventTypeSpecVersionRecord = {
				id: specVersion.id,
				eventTypeId: specVersion.eventTypeId,
				version: specVersion.version,
				mimeType: specVersion.mimeType,
				schemaContent: specVersion.schemaContent,
				schemaType: specVersion.schemaType,
				status: specVersion.status,
				createdAt: specVersion.createdAt ?? now,
				updatedAt: specVersion.updatedAt ?? now,
			};

			await db(tx).insert(eventTypeSpecVersions).values(record);

			const [inserted] = await db(tx)
				.select()
				.from(eventTypeSpecVersions)
				.where(eq(eventTypeSpecVersions.id, specVersion.id))
				.limit(1);

			return recordToSpecVersion(inserted!);
		},

		async updateSpecVersion(specVersion: SpecVersion, tx?: TransactionContext): Promise<SpecVersion> {
			const now = new Date();

			await db(tx)
				.update(eventTypeSpecVersions)
				.set({
					mimeType: specVersion.mimeType,
					schemaContent: specVersion.schemaContent,
					schemaType: specVersion.schemaType,
					status: specVersion.status,
					updatedAt: now,
				})
				.where(eq(eventTypeSpecVersions.id, specVersion.id));

			const [updated] = await db(tx)
				.select()
				.from(eventTypeSpecVersions)
				.where(eq(eventTypeSpecVersions.id, specVersion.id))
				.limit(1);

			return recordToSpecVersion(updated!);
		},

		async findSpecVersionsByEventTypeId(eventTypeId: string, tx?: TransactionContext): Promise<SpecVersion[]> {
			return loadSpecVersions(eventTypeId, tx);
		},
	};
}

/**
 * Convert a database record to an EventType domain entity.
 */
function recordToEventType(record: EventTypeRecord, specVersions: SpecVersion[]): EventType {
	return {
		id: record.id,
		code: record.code,
		name: record.name,
		description: record.description,
		specVersions,
		status: record.status as EventTypeStatus,
		source: record.source as EventTypeSource,
		clientScoped: record.clientScoped,
		application: record.application,
		subdomain: record.subdomain,
		aggregate: record.aggregate,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}

/**
 * Convert a database record to a SpecVersion.
 */
function recordToSpecVersion(record: EventTypeSpecVersionRecord): SpecVersion {
	return {
		id: record.id,
		eventTypeId: record.eventTypeId,
		version: record.version,
		mimeType: record.mimeType,
		schemaContent: record.schemaContent,
		schemaType: record.schemaType as SchemaType,
		status: record.status as SpecVersionStatus,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
