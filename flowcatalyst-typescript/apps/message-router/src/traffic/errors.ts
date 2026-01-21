/**
 * Traffic management error types using discriminated unions for neverthrow
 */

/**
 * Traffic management errors
 */
export type TrafficError =
	| { type: 'registration_failed'; strategy: string; cause: Error }
	| { type: 'deregistration_failed'; strategy: string; cause: Error }
	| { type: 'alb_api_error'; operation: string; statusCode: number; message: string }
	| { type: 'target_not_found'; targetGroupArn: string; targetId: string }
	| { type: 'timeout'; operation: string; durationMs: number }
	| { type: 'configuration_error'; message: string };

/**
 * Helper to create traffic errors
 */
export const TrafficErrors = {
	registrationFailed: (strategy: string, cause: Error): TrafficError => ({
		type: 'registration_failed',
		strategy,
		cause,
	}),
	deregistrationFailed: (strategy: string, cause: Error): TrafficError => ({
		type: 'deregistration_failed',
		strategy,
		cause,
	}),
	albApiError: (operation: string, statusCode: number, message: string): TrafficError => ({
		type: 'alb_api_error',
		operation,
		statusCode,
		message,
	}),
	targetNotFound: (targetGroupArn: string, targetId: string): TrafficError => ({
		type: 'target_not_found',
		targetGroupArn,
		targetId,
	}),
	timeout: (operation: string, durationMs: number): TrafficError => ({
		type: 'timeout',
		operation,
		durationMs,
	}),
	configurationError: (message: string): TrafficError => ({
		type: 'configuration_error',
		message,
	}),
};
