import { describe, it, expect } from 'vitest';
import { TracingContext } from '../tracing-context.js';

describe('TracingContext', () => {
	describe('runWithContext', () => {
		it('should provide correlation ID within context', () => {
			TracingContext.runWithContext('corr-123', null, () => {
				expect(TracingContext.getCorrelationId()).toBe('corr-123');
			});
		});

		it('should provide causation ID within context', () => {
			TracingContext.runWithContext('corr-123', 'cause-456', () => {
				expect(TracingContext.getCausationId()).toBe('cause-456');
			});
		});

		it('should return function result', () => {
			const result = TracingContext.runWithContext('corr', null, () => {
				return 'result';
			});
			expect(result).toBe('result');
		});

		it('should not leak context outside', () => {
			TracingContext.runWithContext('corr-inside', null, () => {
				// Context is available inside
				expect(TracingContext.getCorrelationId()).toBe('corr-inside');
			});
			// Context is not available outside (will generate new ID)
			const outsideCorr = TracingContext.getCorrelationId();
			expect(outsideCorr).not.toBe('corr-inside');
			expect(outsideCorr).toMatch(/^trace-/);
		});
	});

	describe('runWithContextAsync', () => {
		it('should work with async functions', async () => {
			const result = await TracingContext.runWithContextAsync('corr-async', 'cause-async', async () => {
				// Simulate async work
				await new Promise((r) => setTimeout(r, 10));
				return {
					corr: TracingContext.getCorrelationId(),
					cause: TracingContext.getCausationId(),
				};
			});

			expect(result.corr).toBe('corr-async');
			expect(result.cause).toBe('cause-async');
		});
	});

	describe('current / requireCurrent', () => {
		it('should return null outside context', () => {
			expect(TracingContext.current()).toBeNull();
		});

		it('should return context data inside context', () => {
			TracingContext.runWithContext('corr', 'cause', () => {
				const ctx = TracingContext.current();
				expect(ctx).not.toBeNull();
				expect(ctx?.correlationId).toBe('corr');
				expect(ctx?.causationId).toBe('cause');
			});
		});

		it('should throw from requireCurrent outside context', () => {
			expect(() => TracingContext.requireCurrent()).toThrow('No TracingContext available');
		});

		it('should return context from requireCurrent inside context', () => {
			TracingContext.runWithContext('corr', null, () => {
				const ctx = TracingContext.requireCurrent();
				expect(ctx.correlationId).toBe('corr');
			});
		});
	});

	describe('hasCorrelationId / hasCausationId', () => {
		it('should return false outside context', () => {
			// Note: These will be false since there's no context
			// but getCorrelationId() will generate one
			expect(TracingContext.hasCausationId()).toBe(false);
		});

		it('should return true when set', () => {
			TracingContext.runWithContext('corr', 'cause', () => {
				expect(TracingContext.hasCorrelationId()).toBe(true);
				expect(TracingContext.hasCausationId()).toBe(true);
			});
		});

		it('should return false for null causation', () => {
			TracingContext.runWithContext('corr', null, () => {
				expect(TracingContext.hasCorrelationId()).toBe(true);
				expect(TracingContext.hasCausationId()).toBe(false);
			});
		});
	});

	describe('fromHeaders', () => {
		it('should extract correlation ID from headers', () => {
			const ctx = TracingContext.fromHeaders({
				'X-Correlation-ID': 'corr-from-header',
			});
			expect(ctx.correlationId).toBe('corr-from-header');
		});

		it('should extract causation ID from headers', () => {
			const ctx = TracingContext.fromHeaders({
				'X-Causation-ID': 'cause-from-header',
			});
			expect(ctx.causationId).toBe('cause-from-header');
		});

		it('should handle lowercase headers', () => {
			const ctx = TracingContext.fromHeaders({
				'x-correlation-id': 'corr-lower',
				'x-causation-id': 'cause-lower',
			});
			expect(ctx.correlationId).toBe('corr-lower');
			expect(ctx.causationId).toBe('cause-lower');
		});

		it('should handle array header values', () => {
			const ctx = TracingContext.fromHeaders({
				'X-Correlation-ID': ['corr-1', 'corr-2'],
			});
			expect(ctx.correlationId).toBe('corr-1');
		});

		it('should return null for missing headers', () => {
			const ctx = TracingContext.fromHeaders({});
			expect(ctx.correlationId).toBeNull();
			expect(ctx.causationId).toBeNull();
		});
	});

	describe('toHeaders', () => {
		it('should include correlation ID in headers', () => {
			TracingContext.runWithContext('corr-out', null, () => {
				const headers = TracingContext.toHeaders();
				expect(headers['X-Correlation-ID']).toBe('corr-out');
			});
		});

		it('should include causation ID when present', () => {
			TracingContext.runWithContext('corr', 'cause-out', () => {
				const headers = TracingContext.toHeaders();
				expect(headers['X-Causation-ID']).toBe('cause-out');
			});
		});

		it('should not include causation ID when null', () => {
			TracingContext.runWithContext('corr', null, () => {
				const headers = TracingContext.toHeaders();
				expect(headers['X-Causation-ID']).toBeUndefined();
			});
		});
	});

	describe('runFromEvent', () => {
		it('should set correlation and causation from event', () => {
			TracingContext.runFromEvent('event-corr', 'event-123', () => {
				expect(TracingContext.getCorrelationId()).toBe('event-corr');
				expect(TracingContext.getCausationId()).toBe('event-123');
			});
		});
	});

	describe('deriveContext', () => {
		it('should preserve correlation and set new causation', () => {
			TracingContext.runWithContext('original-corr', null, () => {
				const derived = TracingContext.deriveContext('new-cause');
				expect(derived.correlationId).toBe('original-corr');
				expect(derived.causationId).toBe('new-cause');
			});
		});

		it('should generate correlation if not in context', () => {
			const derived = TracingContext.deriveContext('cause');
			expect(derived.correlationId).toMatch(/^trace-/);
			expect(derived.causationId).toBe('cause');
		});
	});
});
