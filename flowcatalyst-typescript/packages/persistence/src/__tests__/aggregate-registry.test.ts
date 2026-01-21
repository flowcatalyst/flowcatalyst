import { describe, it, expect, vi } from 'vitest';
import {
	createAggregateRegistry,
	createAggregateHandler,
	tagAggregate,
	isTaggedAggregate,
} from '../aggregate-registry.js';
import type { TransactionContext } from '../transaction.js';
import type { BaseEntity, NewEntity } from '../schema/common.js';

interface TestEntity extends BaseEntity {
	name: string;
}

describe('AggregateRegistry', () => {
	const mockTx: TransactionContext = { db: {} as never };

	describe('createAggregateRegistry', () => {
		it('should register and dispatch to handlers', async () => {
			const registry = createAggregateRegistry();

			const persistFn = vi.fn().mockResolvedValue({
				id: 'test-id',
				name: 'Test',
				createdAt: new Date(),
				updatedAt: new Date(),
			});

			const handler = {
				typeName: 'TestEntity',
				persist: persistFn,
				delete: vi.fn().mockResolvedValue(true),
				extractId: (agg: TestEntity | NewEntity<TestEntity>) => agg.id,
			};

			registry.register(handler);

			const aggregate = tagAggregate('TestEntity', { id: 'test-id', name: 'Test' });
			await registry.persist(aggregate as never, mockTx);

			expect(persistFn).toHaveBeenCalled();
		});

		it('should throw for unregistered type', async () => {
			const registry = createAggregateRegistry();

			const aggregate = tagAggregate('UnknownType', { id: 'test-id' });

			await expect(registry.persist(aggregate as never, mockTx)).rejects.toThrow(
				'No handler registered for aggregate type: UnknownType',
			);
		});

		it('should extract id from aggregate', () => {
			const registry = createAggregateRegistry();
			const aggregate = { id: 'test-123', name: 'Test' };

			const id = registry.extractId(aggregate as BaseEntity);
			expect(id).toBe('test-123');
		});

		it('should extract id from tagged aggregate', () => {
			const registry = createAggregateRegistry();
			const aggregate = tagAggregate('TestEntity', { id: 'tagged-456', name: 'Test' });

			const id = registry.extractId(aggregate as never);
			expect(id).toBe('tagged-456');
		});
	});

	describe('createAggregateHandler', () => {
		it('should create handler from repository', async () => {
			const repository = {
				persist: vi.fn().mockResolvedValue({
					id: 'test-id',
					name: 'Persisted',
					createdAt: new Date(),
					updatedAt: new Date(),
				}),
				delete: vi.fn().mockResolvedValue(true),
			};

			const handler = createAggregateHandler<TestEntity>('TestEntity', repository);

			expect(handler.typeName).toBe('TestEntity');

			const result = await handler.persist({ id: 'test-id', name: 'Test' }, mockTx);
			expect(result.name).toBe('Persisted');

			const deleted = await handler.delete(
				{ id: 'test-id', name: 'Test', createdAt: new Date(), updatedAt: new Date() },
				mockTx,
			);
			expect(deleted).toBe(true);
		});

		it('should extract id from entity', () => {
			const handler = createAggregateHandler<TestEntity>('TestEntity', {
				persist: vi.fn(),
				delete: vi.fn(),
			});

			const id = handler.extractId({ id: 'entity-789', name: 'Test' });
			expect(id).toBe('entity-789');
		});
	});

	describe('tagAggregate', () => {
		it('should create tagged aggregate', () => {
			const tagged = tagAggregate('MyType', { id: 'id-1', value: 42 });

			expect(tagged._aggregateType).toBe('MyType');
			expect(tagged.aggregate).toEqual({ id: 'id-1', value: 42 });
		});
	});

	describe('isTaggedAggregate', () => {
		it('should return true for tagged aggregates', () => {
			const tagged = tagAggregate('Type', { id: '1' });
			expect(isTaggedAggregate(tagged)).toBe(true);
		});

		it('should return false for plain objects', () => {
			expect(isTaggedAggregate({ id: '1' })).toBe(false);
			expect(isTaggedAggregate(null)).toBe(false);
			expect(isTaggedAggregate(undefined)).toBe(false);
			expect(isTaggedAggregate('string')).toBe(false);
		});
	});
});
