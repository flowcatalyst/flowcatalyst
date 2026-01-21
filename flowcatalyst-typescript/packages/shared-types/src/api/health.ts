import { z } from 'zod';

/**
 * GET /health/live - Liveness probe response
 * GET /health/ready - Readiness probe response
 * GET /health/startup - Startup probe response
 */
export const HealthCheckResponseSchema = z.object({
	status: z.string(),
	timestamp: z.string(),
	issues: z.array(z.string()),
});

export type HealthCheckResponse = z.infer<typeof HealthCheckResponseSchema>;

/**
 * GET /monitoring/health - System health response
 */
export const MonitoringHealthResponseSchema = z.object({
	status: z.string(),
	timestamp: z.string(),
	uptimeMillis: z.number(),
	details: z.object({
		totalQueues: z.number().int(),
		healthyQueues: z.number().int(),
		totalPools: z.number().int(),
		healthyPools: z.number().int(),
		activeWarnings: z.number().int(),
		criticalWarnings: z.number().int(),
		circuitBreakersOpen: z.number().int(),
		degradationReason: z.string().nullable(),
	}),
});

export type MonitoringHealthResponse = z.infer<typeof MonitoringHealthResponseSchema>;
