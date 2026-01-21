/**
 * Repository Interface
 *
 * Base repository interface for CRUD operations on entities.
 * Implementations handle the mapping between domain entities and database records.
 */

import type { BaseEntity, NewEntity } from './schema/common.js';
import type { TransactionContext } from './transaction.js';

/**
 * Base repository interface for entity persistence.
 *
 * @typeParam T - The entity type (must extend BaseEntity)
 */
export interface Repository<T extends BaseEntity> {
	/**
	 * Find an entity by its ID.
	 *
	 * @param id - The entity ID (TSID)
	 * @param tx - Optional transaction context
	 * @returns The entity if found, undefined otherwise
	 */
	findById(id: string, tx?: TransactionContext): Promise<T | undefined>;

	/**
	 * Find all entities.
	 *
	 * @param tx - Optional transaction context
	 * @returns Array of all entities
	 */
	findAll(tx?: TransactionContext): Promise<T[]>;

	/**
	 * Count all entities.
	 *
	 * @param tx - Optional transaction context
	 * @returns Total count
	 */
	count(tx?: TransactionContext): Promise<number>;

	/**
	 * Check if an entity exists by ID.
	 *
	 * @param id - The entity ID (TSID)
	 * @param tx - Optional transaction context
	 * @returns True if entity exists
	 */
	exists(id: string, tx?: TransactionContext): Promise<boolean>;

	/**
	 * Insert a new entity.
	 *
	 * @param entity - The entity to insert
	 * @param tx - Optional transaction context
	 * @returns The inserted entity with generated fields
	 */
	insert(entity: NewEntity<T>, tx?: TransactionContext): Promise<T>;

	/**
	 * Update an existing entity.
	 *
	 * @param entity - The entity to update (must include id)
	 * @param tx - Optional transaction context
	 * @returns The updated entity
	 */
	update(entity: T, tx?: TransactionContext): Promise<T>;

	/**
	 * Insert or update an entity (upsert).
	 * If entity exists (by id), update it. Otherwise, insert it.
	 *
	 * @param entity - The entity to persist
	 * @param tx - Optional transaction context
	 * @returns The persisted entity
	 */
	persist(entity: NewEntity<T>, tx?: TransactionContext): Promise<T>;

	/**
	 * Delete an entity by ID.
	 *
	 * @param id - The entity ID to delete
	 * @param tx - Optional transaction context
	 * @returns True if entity was deleted, false if not found
	 */
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;

	/**
	 * Delete an entity.
	 *
	 * @param entity - The entity to delete
	 * @param tx - Optional transaction context
	 * @returns True if entity was deleted
	 */
	delete(entity: T, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Repository that supports paginated queries.
 */
export interface PaginatedRepository<T extends BaseEntity> extends Repository<T> {
	/**
	 * Find entities with pagination.
	 *
	 * @param page - Page number (0-indexed)
	 * @param pageSize - Number of items per page
	 * @param tx - Optional transaction context
	 * @returns Paginated result
	 */
	findPaged(page: number, pageSize: number, tx?: TransactionContext): Promise<PagedResult<T>>;
}

/**
 * Paginated result with metadata.
 */
export interface PagedResult<T> {
	readonly items: T[];
	readonly page: number;
	readonly pageSize: number;
	readonly totalItems: number;
	readonly totalPages: number;
	readonly hasNext: boolean;
	readonly hasPrevious: boolean;
}

/**
 * Create a paginated result from items and counts.
 */
export function createPagedResult<T>(items: T[], page: number, pageSize: number, totalItems: number): PagedResult<T> {
	const totalPages = Math.ceil(totalItems / pageSize);
	return {
		items,
		page,
		pageSize,
		totalItems,
		totalPages,
		hasNext: page < totalPages - 1,
		hasPrevious: page > 0,
	};
}
