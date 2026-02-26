/**
 * Queue configuration in /api/config response
 */
export interface QueueConfigDto {
	queueUri: string;
	queueName: string | null;
	connections: number | null;
}

/**
 * Processing pool in /api/config response
 */
export interface ProcessingPoolDto {
	code: string;
	concurrency: number;
	rateLimitPerMinute: number | null;
}

/**
 * GET /api/config - Local configuration response
 */
export interface LocalConfigResponse {
	queues: QueueConfigDto[];
	connections: number;
	processingPools: ProcessingPoolDto[];
}
