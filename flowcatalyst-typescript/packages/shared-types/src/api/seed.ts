import { z } from 'zod';

/**
 * POST /api/seed/messages - Request body
 */
export const SeedMessageRequestSchema = z.object({
	count: z.number().int().default(10),
	queue: z.string().default('random'),
	endpoint: z.string().default('random'),
	messageGroupMode: z.string().default('1of8'),
});

export type SeedMessageRequest = z.infer<typeof SeedMessageRequestSchema>;

/**
 * POST /api/seed/messages - Response body
 */
export const SeedMessageResponseSchema = z.object({
	status: z.string(),
	messagesSent: z.number().int().nullable().optional(),
	totalRequested: z.number().int().nullable().optional(),
	message: z.string().nullable().optional(),
});

export type SeedMessageResponse = z.infer<typeof SeedMessageResponseSchema>;
