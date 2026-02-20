/**
 * Test endpoint response (fast, slow, faulty, fail)
 */
export interface TestEndpointResponse {
	status: string;
	endpoint: string;
	requestId: number;
	messageId?: string | undefined;
	error?: string | undefined;
}

/**
 * Mediation response format (success, pending endpoints)
 */
export interface MediationResponse {
	ack: boolean;
	message: string;
	delaySeconds?: number;
}

/**
 * GET /api/test/stats - Response
 */
export interface TestStatsResponse {
	totalRequests: number;
}

/**
 * POST /api/test/stats/reset - Response
 */
export interface TestStatsResetResponse {
	previousCount: number;
	currentCount: number;
}
