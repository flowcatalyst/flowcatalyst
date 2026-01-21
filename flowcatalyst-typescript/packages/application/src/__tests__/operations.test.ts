import { describe, it, expect, vi } from 'vitest';
import {
	Result,
	ExecutionContext,
	BaseDomainEvent,
	DomainEvent,
	RESULT_SUCCESS_TOKEN,
} from '@flowcatalyst/domain-core';
import { createOperationsService, wrapUseCase } from '../operations.js';
import type { UseCase } from '../use-case.js';
import type { Command } from '../command.js';

interface TestCommand extends Command {
	value: string;
}

interface TestEventData {
	value: string;
}

class TestEvent extends BaseDomainEvent<TestEventData> {
	constructor(ctx: ExecutionContext, data: TestEventData) {
		super(
			{
				eventType: 'test:domain:entity:created',
				specVersion: '1.0',
				source: 'test:domain',
				subject: DomainEvent.subject('test', 'entity', 'test-id'),
				messageGroup: DomainEvent.messageGroup('test', 'entity', 'test-id'),
			},
			ctx,
			data,
		);
	}
}

describe('Operations', () => {
	const ctx = ExecutionContext.create('test-principal');

	describe('wrapUseCase', () => {
		it('should wrap use case execute method', async () => {
			const mockExecute = vi.fn().mockResolvedValue(
				Result.success(RESULT_SUCCESS_TOKEN, new TestEvent(ctx, { value: 'test' })),
			);

			const useCase: UseCase<TestCommand, TestEvent> = {
				execute: mockExecute,
			};

			const operation = wrapUseCase(useCase);
			const command: TestCommand = { value: 'test' };

			await operation(command, ctx);

			expect(mockExecute).toHaveBeenCalledWith(command, ctx);
		});

		it('should return use case result', async () => {
			const event = new TestEvent(ctx, { value: 'result' });
			const useCase: UseCase<TestCommand, TestEvent> = {
				execute: vi.fn().mockResolvedValue(Result.success(RESULT_SUCCESS_TOKEN, event)),
			};

			const operation = wrapUseCase(useCase);
			const result = await operation({ value: 'input' }, ctx);

			expect(Result.isSuccess(result)).toBe(true);
			if (Result.isSuccess(result)) {
				expect(result.value.getData().value).toBe('result');
			}
		});
	});

	describe('createOperationsService', () => {
		it('should build service with write operations', async () => {
			const createUseCase: UseCase<TestCommand, TestEvent> = {
				execute: vi.fn().mockResolvedValue(
					Result.success(RESULT_SUCCESS_TOKEN, new TestEvent(ctx, { value: 'created' })),
				),
			};

			const updateUseCase: UseCase<TestCommand, TestEvent> = {
				execute: vi.fn().mockResolvedValue(
					Result.success(RESULT_SUCCESS_TOKEN, new TestEvent(ctx, { value: 'updated' })),
				),
			};

			const ops = createOperationsService()
				.write('create', createUseCase)
				.write('update', updateUseCase)
				.build();

			const createResult = await ops.create({ value: 'new' }, ctx);
			const updateResult = await ops.update({ value: 'change' }, ctx);

			expect(Result.isSuccess(createResult)).toBe(true);
			expect(Result.isSuccess(updateResult)).toBe(true);
		});

		it('should build service with read operations', async () => {
			const ops = createOperationsService()
				.read('findById', async (id: string) => ({ id, name: 'Test' }))
				.read('findAll', async () => [{ id: '1' }, { id: '2' }])
				.build();

			const item = await ops.findById('test-123');
			const all = await ops.findAll();

			expect(item).toEqual({ id: 'test-123', name: 'Test' });
			expect(all).toHaveLength(2);
		});

		it('should build service with sync read operations', () => {
			const ops = createOperationsService()
				.syncRead('count', () => 42)
				.syncRead('exists', (id: string) => id === 'test')
				.build();

			expect(ops.count()).toBe(42);
			expect(ops.exists('test')).toBe(true);
			expect(ops.exists('other')).toBe(false);
		});

		it('should build mixed service', async () => {
			const createUseCase: UseCase<TestCommand, TestEvent> = {
				execute: vi.fn().mockResolvedValue(
					Result.success(RESULT_SUCCESS_TOKEN, new TestEvent(ctx, { value: 'created' })),
				),
			};

			const ops = createOperationsService()
				.write('create', createUseCase)
				.read('findById', async (id: string) => ({ id }))
				.syncRead('count', () => 10)
				.build();

			const createResult = await ops.create({ value: 'new' }, ctx);
			const found = await ops.findById('123');
			const count = ops.count();

			expect(Result.isSuccess(createResult)).toBe(true);
			expect(found).toEqual({ id: '123' });
			expect(count).toBe(10);
		});
	});
});
