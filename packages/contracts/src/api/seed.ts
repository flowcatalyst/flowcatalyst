/**
 * POST /api/seed/messages - Request body
 */
export interface SeedMessageRequest {
	count?: number | undefined;
	queue?: string | undefined;
	endpoint?: string | undefined;
	messageGroupMode?: string | undefined;
}

/**
 * POST /api/seed/messages - Response body
 */
export interface SeedMessageResponse {
	status: string;
	messagesSent?: number | undefined;
	totalRequested?: number | undefined;
	message?: string | undefined;
}
