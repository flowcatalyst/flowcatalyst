/**
 * Aggregate Registry
 *
 * Central dispatcher for persisting and deleting aggregates.
 * Maps aggregate types to their respective repository operations.
 */

import type { BaseEntity, NewEntity } from './schema/common.js';
import type { TransactionContext } from './transaction.js';

/**
 * Handler for persisting and deleting a specific aggregate type.
 */
export interface AggregateHandler<T extends BaseEntity = BaseEntity> {
	/**
	 * Get the aggregate type name.
	 */
	readonly typeName: string;

	/**
	 * Persist (insert or update) an aggregate.
	 */
	persist(aggregate: NewEntity<T>, tx: TransactionContext): Promise<T>;

	/**
	 * Delete an aggregate.
	 */
	delete(aggregate: T, tx: TransactionContext): Promise<boolean>;

	/**
	 * Extract the ID from an aggregate.
	 */
	extractId(aggregate: T | NewEntity<T>): string;
}

/**
 * Registry for aggregate handlers.
 * Dispatches persist/delete operations to the appropriate handler based on aggregate type.
 */
export interface AggregateRegistry {
	/**
	 * Register a handler for an aggregate type.
	 *
	 * @param handler - The aggregate handler
	 */
	register<T extends BaseEntity>(handler: AggregateHandler<T>): void;

	/**
	 * Persist an aggregate using its registered handler.
	 *
	 * @param aggregate - The aggregate to persist
	 * @param tx - Transaction context
	 * @returns The persisted aggregate
	 * @throws Error if no handler is registered for the aggregate type
	 */
	persist<T extends BaseEntity>(aggregate: NewEntity<T>, tx: TransactionContext): Promise<T>;

	/**
	 * Delete an aggregate using its registered handler.
	 *
	 * @param aggregate - The aggregate to delete
	 * @param tx - Transaction context
	 * @returns True if deleted
	 * @throws Error if no handler is registered for the aggregate type
	 */
	delete<T extends BaseEntity>(aggregate: T, tx: TransactionContext): Promise<boolean>;

	/**
	 * Extract the ID from an aggregate.
	 *
	 * @param aggregate - The aggregate
	 * @returns The aggregate ID
	 */
	extractId<T extends BaseEntity>(aggregate: T | NewEntity<T>): string;

	/**
	 * Extract the type name from an aggregate.
	 *
	 * @param aggregate - The aggregate
	 * @returns The type name
	 */
	extractTypeName<T extends BaseEntity>(aggregate: T | NewEntity<T>): string;
}

/**
 * Tagged aggregate - wraps an aggregate with its type information.
 * Use this when you need to persist aggregates of unknown type through the registry.
 */
export interface TaggedAggregate<T extends BaseEntity = BaseEntity> {
	readonly _aggregateType: string;
	readonly aggregate: T | NewEntity<T>;
}

/**
 * Create a tagged aggregate.
 */
export function tagAggregate<T extends BaseEntity>(typeName: string, aggregate: T | NewEntity<T>): TaggedAggregate<T> {
	return { _aggregateType: typeName, aggregate };
}

/**
 * Check if an object is a tagged aggregate.
 */
export function isTaggedAggregate(obj: unknown): obj is TaggedAggregate {
	return typeof obj === 'object' && obj !== null && '_aggregateType' in obj && 'aggregate' in obj;
}

/**
 * Create an aggregate registry.
 *
 * @returns A new aggregate registry
 */
export function createAggregateRegistry(): AggregateRegistry {
	const handlers = new Map<string, AggregateHandler>();

	return {
		register<T extends BaseEntity>(handler: AggregateHandler<T>): void {
			handlers.set(handler.typeName, handler as AggregateHandler);
		},

		async persist<T extends BaseEntity>(aggregate: NewEntity<T>, tx: TransactionContext): Promise<T> {
			const typeName = this.extractTypeName(aggregate);
			const handler = handlers.get(typeName);

			if (!handler) {
				throw new Error(
					`No handler registered for aggregate type: ${typeName}. ` +
						`Registered types: ${Array.from(handlers.keys()).join(', ')}`,
				);
			}

			return handler.persist(aggregate as NewEntity<BaseEntity>, tx) as Promise<T>;
		},

		async delete<T extends BaseEntity>(aggregate: T, tx: TransactionContext): Promise<boolean> {
			const typeName = this.extractTypeName(aggregate);
			const handler = handlers.get(typeName);

			if (!handler) {
				throw new Error(
					`No handler registered for aggregate type: ${typeName}. ` +
						`Registered types: ${Array.from(handlers.keys()).join(', ')}`,
				);
			}

			return handler.delete(aggregate as BaseEntity, tx);
		},

		extractId<T extends BaseEntity>(aggregate: T | NewEntity<T>): string {
			if (isTaggedAggregate(aggregate)) {
				const inner = aggregate.aggregate;
				if ('id' in inner && typeof inner.id === 'string') {
					return inner.id;
				}
				throw new Error('Tagged aggregate does not have an id field');
			}

			if ('id' in aggregate && typeof aggregate.id === 'string') {
				return aggregate.id;
			}

			throw new Error('Aggregate does not have an id field');
		},

		extractTypeName<T extends BaseEntity>(aggregate: T | NewEntity<T>): string {
			if (isTaggedAggregate(aggregate)) {
				return aggregate._aggregateType;
			}

			// Try to get type from constructor name
			const constructor = (aggregate as object).constructor;
			if (constructor && constructor.name && constructor.name !== 'Object') {
				return constructor.name;
			}

			throw new Error(
				'Cannot determine aggregate type. Use tagAggregate() to wrap the aggregate with type information, ' +
					'or use a class instance instead of a plain object.',
			);
		},
	};
}

/**
 * Create an aggregate handler from a repository.
 *
 * @param typeName - The aggregate type name
 * @param repository - The repository with persist and delete operations
 * @returns An aggregate handler
 */
export function createAggregateHandler<T extends BaseEntity>(
	typeName: string,
	repository: {
		persist(entity: NewEntity<T>, tx?: TransactionContext): Promise<T>;
		delete(entity: T, tx?: TransactionContext): Promise<boolean>;
	},
): AggregateHandler<T> {
	return {
		typeName,
		persist: (aggregate, tx) => repository.persist(aggregate, tx),
		delete: (aggregate, tx) => repository.delete(aggregate, tx),
		extractId: (aggregate) => {
			if ('id' in aggregate && typeof aggregate.id === 'string') {
				return aggregate.id;
			}
			throw new Error(`Cannot extract id from ${typeName} aggregate`);
		},
	};
}
