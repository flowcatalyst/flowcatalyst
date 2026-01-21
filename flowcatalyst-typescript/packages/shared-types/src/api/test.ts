import { z } from 'zod';

/**
 * Test endpoint response (fast, slow, faulty, fail)
 */
export const TestEndpointResponseSchema = z.object({
	status: z.string(),
	endpoint: z.string(),
	requestId: z.number().int(),
	messageId: z.string().optional(),
	error: z.string().optional(),
});

export type TestEndpointResponse = z.infer<typeof TestEndpointResponseSchema>;

/**
 * Mediation response format (success, pending endpoints)
 */
export const MediationResponseSchema = z.object({
	ack: z.boolean(),
	message: z.string(),
});

export type MediationResponse = z.infer<typeof MediationResponseSchema>;

/**
 * GET /api/test/stats - Response
 */
export const TestStatsResponseSchema = z.object({
	totalRequests: z.number().int(),
});

export type TestStatsResponse = z.infer<typeof TestStatsResponseSchema>;

/**
 * POST /api/test/stats/reset - Response
 */
export const TestStatsResetResponseSchema = z.object({
	previousCount: z.number().int(),
	currentCount: z.number().int(),
});

export type TestStatsResetResponse = z.infer<typeof TestStatsResetResponseSchema>;
