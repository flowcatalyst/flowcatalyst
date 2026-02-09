import type { FastifyPluginAsync } from 'fastify';
import {
  BenchmarkProcessResponseSchema,
  BenchmarkProcessSlowResponseSchema,
  BenchmarkStatsResponseSchema,
  BenchmarkResetResponseSchema,
  BenchmarkSlowQuerySchema,
} from '../schemas/index.js';

let requestCount = 0;
let startTime = 0;

export const benchmarkRoutes: FastifyPluginAsync = async (fastify) => {
  // POST /benchmark/process
  fastify.post(
    '/process',
    {
      schema: {
        tags: ['Benchmark'],
        summary: 'Fast mock processing endpoint',
        response: { 200: BenchmarkProcessResponseSchema },
      },
    },
    (request) => {
      requestCount++;
      const count = requestCount;
      if (count === 1) {
        startTime = Date.now();
      }
      if (count % 100 === 0) {
        const elapsed = Date.now() - startTime;
        const throughput = count / (elapsed / 1000);
        request.log.info({ count, throughput: throughput.toFixed(2) }, 'Benchmark progress');
      }
      return { status: 'ok', requestId: count, timestamp: Date.now() };
    },
  );

  // POST /benchmark/process-slow
  fastify.post(
    '/process-slow',
    {
      schema: {
        tags: ['Benchmark'],
        summary: 'Slow mock processing endpoint',
        querystring: BenchmarkSlowQuerySchema,
        response: { 200: BenchmarkProcessSlowResponseSchema },
      },
    },
    async (request) => {
      const { delayMs = '100' } = request.query as { delayMs?: string };
      const delay = Number(delayMs);
      await new Promise((resolve) => setTimeout(resolve, delay));
      requestCount++;
      return { status: 'ok', requestId: requestCount, delayMs: delay, timestamp: Date.now() };
    },
  );

  // GET /benchmark/stats
  fastify.get(
    '/stats',
    {
      schema: {
        tags: ['Benchmark'],
        summary: 'Get benchmark statistics',
        response: { 200: BenchmarkStatsResponseSchema },
      },
    },
    () => {
      const elapsed = startTime > 0 ? Date.now() - startTime : 0;
      const throughput = startTime > 0 && elapsed > 0 ? requestCount / (elapsed / 1000) : 0;
      return {
        totalRequests: requestCount,
        elapsedMs: elapsed,
        throughputPerSecond: Math.round(throughput * 100) / 100,
      };
    },
  );

  // POST /benchmark/reset
  fastify.post(
    '/reset',
    {
      schema: {
        tags: ['Benchmark'],
        summary: 'Reset benchmark statistics',
        response: { 200: BenchmarkResetResponseSchema },
      },
    },
    (request) => {
      requestCount = 0;
      startTime = 0;
      request.log.info('Benchmark stats reset');
      return { status: 'reset' };
    },
  );
};
