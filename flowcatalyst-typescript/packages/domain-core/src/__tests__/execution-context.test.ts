import { describe, it, expect } from 'vitest';
import { ExecutionContext } from '../execution-context.js';
import { TracingContext } from '../tracing-context.js';

describe('ExecutionContext', () => {
	describe('create', () => {
		it('should create context with generated execution ID', () => {
			const ctx = ExecutionContext.create('principal-123');

			expect(ctx.executionId).toMatch(/^exec-/);
			expect(ctx.principalId).toBe('principal-123');
			expect(ctx.initiatedAt).toBeInstanceOf(Date);
		});

		it('should use execution ID as correlation ID for fresh requests', () => {
			const ctx = ExecutionContext.create('principal-123');

			// For fresh requests (no TracingContext), correlation = execution
			expect(ctx.correlationId).toBe(ctx.executionId);
			expect(ctx.causationId).toBeNull();
		});

		it('should use TracingContext when available', () => {
			TracingContext.runWithContext('traced-corr', 'traced-cause', () => {
				const ctx = ExecutionContext.create('principal-456');

				expect(ctx.executionId).toMatch(/^exec-/);
				expect(ctx.correlationId).toBe('traced-corr');
				expect(ctx.causationId).toBe('traced-cause');
				expect(ctx.principalId).toBe('principal-456');
			});
		});
	});

	describe('fromTracingContext', () => {
		it('should create context from tracing data', () => {
			const tracingData = { correlationId: 'corr-abc', causationId: 'cause-xyz' };
			const ctx = ExecutionContext.fromTracingContext(tracingData, 'principal-789');

			expect(ctx.executionId).toMatch(/^exec-/);
			expect(ctx.correlationId).toBe('corr-abc');
			expect(ctx.causationId).toBe('cause-xyz');
			expect(ctx.principalId).toBe('principal-789');
		});

		it('should use execution ID as correlation if not provided', () => {
			const tracingData = { correlationId: null, causationId: null };
			const ctx = ExecutionContext.fromTracingContext(tracingData, 'principal');

			expect(ctx.correlationId).toBe(ctx.executionId);
		});
	});

	describe('withCorrelation', () => {
		it('should create context with specific correlation ID', () => {
			const ctx = ExecutionContext.withCorrelation('principal', 'my-corr-id');

			expect(ctx.executionId).toMatch(/^exec-/);
			expect(ctx.correlationId).toBe('my-corr-id');
			expect(ctx.causationId).toBeNull();
			expect(ctx.principalId).toBe('principal');
		});
	});

	describe('fromParentEvent', () => {
		it('should create context linked to parent event', () => {
			const parentEvent = {
				eventId: 'event-parent-123',
				correlationId: 'parent-corr',
				// ... other DomainEvent fields would be here
			};

			const ctx = ExecutionContext.fromParentEvent(parentEvent as never, 'child-principal');

			expect(ctx.executionId).toMatch(/^exec-/);
			expect(ctx.correlationId).toBe('parent-corr');
			expect(ctx.causationId).toBe('event-parent-123');
			expect(ctx.principalId).toBe('child-principal');
		});
	});

	describe('withCausation', () => {
		it('should create child context with same execution ID', () => {
			const parent = ExecutionContext.create('principal');
			const child = ExecutionContext.withCausation(parent, 'causing-event-456');

			expect(child.executionId).toBe(parent.executionId);
			expect(child.correlationId).toBe(parent.correlationId);
			expect(child.causationId).toBe('causing-event-456');
			expect(child.principalId).toBe(parent.principalId);
		});
	});

	describe('unique execution IDs', () => {
		it('should generate unique execution IDs', () => {
			const ids = new Set<string>();
			for (let i = 0; i < 100; i++) {
				const ctx = ExecutionContext.create('principal');
				ids.add(ctx.executionId);
			}
			expect(ids.size).toBe(100);
		});
	});
});
