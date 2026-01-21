import { z } from 'zod';

/**
 * Queue configuration in /api/config response
 */
export const QueueConfigDtoSchema = z.object({
	queueUri: z.string(),
	queueName: z.string().nullable(),
	connections: z.number().int().nullable(),
});

export type QueueConfigDto = z.infer<typeof QueueConfigDtoSchema>;

/**
 * Processing pool in /api/config response
 */
export const ProcessingPoolDtoSchema = z.object({
	code: z.string(),
	concurrency: z.number().int(),
	rateLimitPerMinute: z.number().int().nullable(),
});

export type ProcessingPoolDto = z.infer<typeof ProcessingPoolDtoSchema>;

/**
 * GET /api/config - Local configuration response
 */
export const LocalConfigResponseSchema = z.object({
	queues: z.array(QueueConfigDtoSchema),
	connections: z.number().int(),
	processingPools: z.array(ProcessingPoolDtoSchema),
});

export type LocalConfigResponse = z.infer<typeof LocalConfigResponseSchema>;
