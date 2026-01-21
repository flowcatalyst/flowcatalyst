import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createNoOpUnitOfWork } from '../unit-of-work.js';
import { Result, BaseDomainEvent, DomainEvent, ExecutionContext } from '@flowcatalyst/domain-core';

interface TestEventData {
	testId: string;
	value: string;
}

class TestEvent extends BaseDomainEvent<TestEventData> {
	constructor(ctx: ExecutionContext, data: TestEventData) {
		super(
			{
				eventType: 'test:domain:entity:created',
				specVersion: '1.0',
				source: 'test:domain',
				subject: DomainEvent.subject('test', 'entity', data.testId),
				messageGroup: DomainEvent.messageGroup('test', 'entity', data.testId),
			},
			ctx,
			data,
		);
	}
}

describe('UnitOfWork', () => {
	describe('createNoOpUnitOfWork', () => {
		const ctx = ExecutionContext.create('test-principal');

		it('should return success on commit', async () => {
			const unitOfWork = createNoOpUnitOfWork();
			const event = new TestEvent(ctx, { testId: 'test-1', value: 'test' });
			const aggregate = { id: 'agg-1' };
			const command = { type: 'CreateTest' };

			const result = await unitOfWork.commit(aggregate, event, command);

			expect(Result.isSuccess(result)).toBe(true);
			if (Result.isSuccess(result)) {
				expect(result.value).toBe(event);
			}
		});

		it('should return success on commitDelete', async () => {
			const unitOfWork = createNoOpUnitOfWork();
			const event = new TestEvent(ctx, { testId: 'test-2', value: 'deleted' });
			const aggregate = { id: 'agg-2' };
			const command = { type: 'DeleteTest' };

			const result = await unitOfWork.commitDelete(aggregate, event, command);

			expect(Result.isSuccess(result)).toBe(true);
		});

		it('should return success on commitAll', async () => {
			const unitOfWork = createNoOpUnitOfWork();
			const event = new TestEvent(ctx, { testId: 'test-3', value: 'batch' });
			const aggregates = [{ id: 'agg-3a' }, { id: 'agg-3b' }];
			const command = { type: 'BatchCreate' };

			const result = await unitOfWork.commitAll(aggregates, event, command);

			expect(Result.isSuccess(result)).toBe(true);
		});
	});
});
