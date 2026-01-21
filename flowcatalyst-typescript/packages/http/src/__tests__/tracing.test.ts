import { describe, it, expect } from 'vitest';
import { Hono } from 'hono';
import { tracingMiddleware, requireTracing, getTracingHeaders } from '../middleware/tracing.js';
import type { FlowCatalystEnv } from '../types.js';

describe('Tracing Middleware', () => {
	it('should generate correlation ID when not provided', async () => {
		const app = new Hono<FlowCatalystEnv>();
		app.use('*', tracingMiddleware());
		app.get('/test', (c) => {
			const tracing = c.get('tracing');
			return c.json({
				correlationId: tracing.correlationId,
				executionId: tracing.executionId,
			});
		});

		const res = await app.request('/test');
		const data = await res.json();

		expect(data.correlationId).toMatch(/^trace-/);
		expect(data.executionId).toMatch(/^exec-/);
	});

	it('should use X-Correlation-ID header when provided', async () => {
		const app = new Hono<FlowCatalystEnv>();
		app.use('*', tracingMiddleware());
		app.get('/test', (c) => {
			const tracing = c.get('tracing');
			return c.json({ correlationId: tracing.correlationId });
		});

		const res = await app.request('/test', {
			headers: { 'X-Correlation-ID': 'test-corr-123' },
		});
		const data = await res.json();

		expect(data.correlationId).toBe('test-corr-123');
	});

	it('should use X-Request-ID as fallback', async () => {
		const app = new Hono<FlowCatalystEnv>();
		app.use('*', tracingMiddleware());
		app.get('/test', (c) => {
			const tracing = c.get('tracing');
			return c.json({ correlationId: tracing.correlationId });
		});

		const res = await app.request('/test', {
			headers: { 'X-Request-ID': 'req-456' },
		});
		const data = await res.json();

		expect(data.correlationId).toBe('req-456');
	});

	it('should extract causation ID from header', async () => {
		const app = new Hono<FlowCatalystEnv>();
		app.use('*', tracingMiddleware());
		app.get('/test', (c) => {
			const tracing = c.get('tracing');
			return c.json({ causationId: tracing.causationId });
		});

		const res = await app.request('/test', {
			headers: { 'X-Causation-ID': 'cause-789' },
		});
		const data = await res.json();

		expect(data.causationId).toBe('cause-789');
	});

	it('should add correlation ID to response headers', async () => {
		const app = new Hono<FlowCatalystEnv>();
		app.use('*', tracingMiddleware());
		app.get('/test', (c) => c.text('ok'));

		const res = await app.request('/test', {
			headers: { 'X-Correlation-ID': 'resp-corr' },
		});

		expect(res.headers.get('X-Correlation-ID')).toBe('resp-corr');
	});

	it('should track start time', async () => {
		const app = new Hono<FlowCatalystEnv>();
		app.use('*', tracingMiddleware());
		app.get('/test', (c) => {
			const tracing = c.get('tracing');
			return c.json({ hasStartTime: tracing.startTime > 0 });
		});

		const res = await app.request('/test');
		const data = await res.json();

		expect(data.hasStartTime).toBe(true);
	});

	describe('requireTracing', () => {
		it('should throw when tracing not available', () => {
			const mockContext = { get: () => undefined };
			expect(() => requireTracing(mockContext as never)).toThrow('Tracing context not available');
		});
	});

	describe('getTracingHeaders', () => {
		it('should return headers for outgoing requests', () => {
			const tracing = {
				correlationId: 'corr-123',
				executionId: 'exec-456',
				causationId: null,
				startTime: Date.now(),
			};

			const headers = getTracingHeaders(tracing);

			expect(headers['X-Correlation-ID']).toBe('corr-123');
			expect(headers['X-Causation-ID']).toBe('exec-456');
		});
	});
});
