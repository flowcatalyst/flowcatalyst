import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { createLogger } from '@flowcatalyst/logging';
import type { FastifyInstance } from 'fastify';
import { createApp } from '../app.js';

describe('Message Router API', () => {
  let app: FastifyInstance;

  beforeAll(async () => {
    const logger = createLogger({
      level: 'error',
      serviceName: 'test',
      pretty: false,
    });
    ({ app } = await createApp(logger));
    await app.ready();
  });

  afterAll(async () => {
    await app.close();
  });

  describe('Health endpoints', () => {
    it('GET /health/live returns 200', async () => {
      const res = await app.inject({ method: 'GET', url: '/health/live' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body).toHaveProperty('status');
      expect(body).toHaveProperty('timestamp');
      expect(body).toHaveProperty('issues');
    });

    it('GET /health/ready returns health status', async () => {
      const res = await app.inject({ method: 'GET', url: '/health/ready' });
      // May be 200 or 503 depending on startup state
      expect([200, 503]).toContain(res.statusCode);

      const body = res.json();
      expect(body).toHaveProperty('status');
      expect(body).toHaveProperty('timestamp');
      expect(body).toHaveProperty('issues');
    });

    it('GET /health/startup returns startup status', async () => {
      const res = await app.inject({ method: 'GET', url: '/health/startup' });
      expect([200, 503]).toContain(res.statusCode);

      const body = res.json();
      expect(body).toHaveProperty('status');
    });
  });

  describe('Config endpoint', () => {
    it('GET /api/config returns configuration', async () => {
      const res = await app.inject({ method: 'GET', url: '/api/config' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body).toHaveProperty('queues');
      expect(body).toHaveProperty('connections');
      expect(body).toHaveProperty('processingPools');
      expect(Array.isArray(body.queues)).toBe(true);
      expect(Array.isArray(body.processingPools)).toBe(true);
    });
  });

  describe('Monitoring endpoints', () => {
    it('GET /monitoring/health returns system health', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/health' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body).toHaveProperty('status');
      expect(body).toHaveProperty('timestamp');
      expect(body).toHaveProperty('uptimeMillis');
      expect(body).toHaveProperty('details');
    });

    it('GET /monitoring/queue-stats returns queue statistics', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/queue-stats' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(typeof body).toBe('object');
    });

    it('GET /monitoring/pool-stats returns pool statistics', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/pool-stats' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(typeof body).toBe('object');
    });

    it('GET /monitoring/warnings returns warnings array', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/warnings' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(Array.isArray(body)).toBe(true);
    });

    it('GET /monitoring/circuit-breakers returns circuit breaker stats', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/circuit-breakers' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(typeof body).toBe('object');
    });

    it('GET /monitoring/in-flight-messages returns messages array', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/in-flight-messages' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(Array.isArray(body)).toBe(true);
    });

    it('GET /monitoring/standby-status returns standby info', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/standby-status' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body).toHaveProperty('standbyEnabled');
    });

    it('GET /monitoring/consumer-health returns consumer health', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/consumer-health' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body).toHaveProperty('currentTimeMs');
      expect(body).toHaveProperty('currentTime');
      expect(body).toHaveProperty('consumers');
    });

    it('GET /monitoring/dashboard returns HTML', async () => {
      const res = await app.inject({ method: 'GET', url: '/monitoring/dashboard' });
      // May return 404 if public folder not available in test
      expect([200, 404]).toContain(res.statusCode);

      if (res.statusCode === 200) {
        const html = res.body;
        expect(html).toContain('FlowCatalyst Dashboard');
        expect(html).toContain('<!DOCTYPE html>');
      }
    });
  });

  describe('Test endpoints', () => {
    it('POST /api/test/fast returns success after delay', async () => {
      const res = await app.inject({ method: 'POST', url: '/api/test/fast' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body.status).toBe('success');
      expect(body.endpoint).toBe('fast');
      expect(body).toHaveProperty('requestId');
    });

    it('POST /api/test/fail returns 500', async () => {
      const res = await app.inject({ method: 'POST', url: '/api/test/fail' });
      expect(res.statusCode).toBe(500);

      const body = res.json();
      expect(body.status).toBe('error');
      expect(body.endpoint).toBe('fail');
    });

    it('POST /api/test/success returns mediation response', async () => {
      const res = await app.inject({ method: 'POST', url: '/api/test/success' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body).toHaveProperty('ack');
      expect(body.ack).toBe(true);
      expect(body).toHaveProperty('message');
    });

    it('POST /api/test/pending returns mediation pending response', async () => {
      const res = await app.inject({ method: 'POST', url: '/api/test/pending' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body.ack).toBe(false);
      expect(body.message).toBe('notBefore time not reached');
    });

    it('GET /api/test/stats returns request count', async () => {
      const res = await app.inject({ method: 'GET', url: '/api/test/stats' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body).toHaveProperty('totalRequests');
      expect(typeof body.totalRequests).toBe('number');
    });
  });

  describe('OpenAPI endpoint', () => {
    it('GET /docs/json returns OpenAPI spec', async () => {
      const res = await app.inject({ method: 'GET', url: '/docs/json' });
      expect(res.statusCode).toBe(200);

      const body = res.json();
      expect(body).toHaveProperty('openapi');
      expect(body).toHaveProperty('info');
      expect(body).toHaveProperty('paths');
    });
  });
});
