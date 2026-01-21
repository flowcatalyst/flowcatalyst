import { describe, it, expect, beforeAll } from 'vitest';
import { createLogger } from '@flowcatalyst/logging';
import { createApp } from '../app.js';

describe('Message Router API', () => {
	let app: ReturnType<typeof createApp>;

	beforeAll(() => {
		const logger = createLogger({
			level: 'error',
			serviceName: 'test',
			pretty: false,
		});
		app = createApp(logger);
	});

	describe('Health endpoints', () => {
		it('GET /health/live returns 200', async () => {
			const res = await app.request('/health/live');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(body).toHaveProperty('status');
			expect(body).toHaveProperty('timestamp');
			expect(body).toHaveProperty('issues');
		});

		it('GET /health/ready returns health status', async () => {
			const res = await app.request('/health/ready');
			// May be 200 or 503 depending on startup state
			expect([200, 503]).toContain(res.status);

			const body = await res.json();
			expect(body).toHaveProperty('status');
			expect(body).toHaveProperty('timestamp');
			expect(body).toHaveProperty('issues');
		});

		it('GET /health/startup returns startup status', async () => {
			const res = await app.request('/health/startup');
			expect([200, 503]).toContain(res.status);

			const body = await res.json();
			expect(body).toHaveProperty('status');
		});
	});

	describe('Config endpoint', () => {
		it('GET /api/config returns configuration', async () => {
			const res = await app.request('/api/config');
			expect(res.status).toBe(200);

			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const body = (await res.json()) as any;
			expect(body).toHaveProperty('queues');
			expect(body).toHaveProperty('connections');
			expect(body).toHaveProperty('processingPools');
			expect(Array.isArray(body.queues)).toBe(true);
			expect(Array.isArray(body.processingPools)).toBe(true);
		});
	});

	describe('Monitoring endpoints', () => {
		it('GET /monitoring/health returns system health', async () => {
			const res = await app.request('/monitoring/health');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(body).toHaveProperty('status');
			expect(body).toHaveProperty('timestamp');
			expect(body).toHaveProperty('uptimeMillis');
			expect(body).toHaveProperty('details');
		});

		it('GET /monitoring/queue-stats returns queue statistics', async () => {
			const res = await app.request('/monitoring/queue-stats');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(typeof body).toBe('object');
		});

		it('GET /monitoring/pool-stats returns pool statistics', async () => {
			const res = await app.request('/monitoring/pool-stats');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(typeof body).toBe('object');
		});

		it('GET /monitoring/warnings returns warnings array', async () => {
			const res = await app.request('/monitoring/warnings');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(Array.isArray(body)).toBe(true);
		});

		it('GET /monitoring/circuit-breakers returns circuit breaker stats', async () => {
			const res = await app.request('/monitoring/circuit-breakers');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(typeof body).toBe('object');
		});

		it('GET /monitoring/in-flight-messages returns messages array', async () => {
			const res = await app.request('/monitoring/in-flight-messages');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(Array.isArray(body)).toBe(true);
		});

		it('GET /monitoring/standby-status returns standby info', async () => {
			const res = await app.request('/monitoring/standby-status');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(body).toHaveProperty('standbyEnabled');
		});

		it('GET /monitoring/consumer-health returns consumer health', async () => {
			const res = await app.request('/monitoring/consumer-health');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(body).toHaveProperty('currentTimeMs');
			expect(body).toHaveProperty('currentTime');
			expect(body).toHaveProperty('consumers');
		});

		it('GET /monitoring/dashboard returns HTML', async () => {
			const res = await app.request('/monitoring/dashboard');
			// May return 404 if public folder not available in test
			expect([200, 404]).toContain(res.status);

			if (res.status === 200) {
				const html = await res.text();
				expect(html).toContain('FlowCatalyst Dashboard');
				expect(html).toContain('<!DOCTYPE html>');
			}
		});
	});

	describe('Test endpoints', () => {
		it('POST /api/test/fast returns success after delay', async () => {
			const res = await app.request('/api/test/fast', { method: 'POST' });
			expect(res.status).toBe(200);

			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const body = (await res.json()) as any;
			expect(body.status).toBe('success');
			expect(body.endpoint).toBe('fast');
			expect(body).toHaveProperty('requestId');
		});

		it('POST /api/test/fail returns 500', async () => {
			const res = await app.request('/api/test/fail', { method: 'POST' });
			expect(res.status).toBe(500);

			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const body = (await res.json()) as any;
			expect(body.status).toBe('error');
			expect(body.endpoint).toBe('fail');
		});

		it('POST /api/test/success returns mediation response', async () => {
			const res = await app.request('/api/test/success', { method: 'POST' });
			expect(res.status).toBe(200);

			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const body = (await res.json()) as any;
			expect(body).toHaveProperty('ack');
			expect(body.ack).toBe(true);
			expect(body).toHaveProperty('message');
		});

		it('POST /api/test/pending returns mediation pending response', async () => {
			const res = await app.request('/api/test/pending', { method: 'POST' });
			expect(res.status).toBe(200);

			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const body = (await res.json()) as any;
			expect(body.ack).toBe(false);
			expect(body.message).toBe('notBefore time not reached');
		});

		it('GET /api/test/stats returns request count', async () => {
			const res = await app.request('/api/test/stats');
			expect(res.status).toBe(200);

			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const body = (await res.json()) as any;
			expect(body).toHaveProperty('totalRequests');
			expect(typeof body.totalRequests).toBe('number');
		});
	});

	describe('OpenAPI endpoint', () => {
		it('GET /openapi.json returns OpenAPI spec', async () => {
			const res = await app.request('/openapi.json');
			expect(res.status).toBe(200);

			const body = await res.json();
			expect(body).toHaveProperty('openapi');
			expect(body).toHaveProperty('info');
			expect(body).toHaveProperty('paths');
		});
	});
});
