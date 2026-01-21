import { z } from 'zod';

/**
 * Message pointer schema - the envelope containing routing info
 * Matches the Java MessagePointer class structure
 */
export const MessagePointerSchema = z.object({
	/** Unique message ID from the application */
	messageId: z.string(),
	/** Pool code for routing */
	poolCode: z.string(),
	/** Message group ID for FIFO ordering within a group */
	messageGroupId: z.string(),
	/** The actual message payload (opaque to router) */
	payload: z.unknown().optional(),
	/** Optional auth token for downstream calls */
	authToken: z.string().optional(),
	/** Optional callback URL override */
	callbackUrl: z.string().optional(),
	/** Timestamp when message was created */
	createdAt: z.string().optional(),
});

/** MessagePointer type with undefined-friendly optional props for exactOptionalPropertyTypes */
export type MessagePointer = {
	messageId: string;
	poolCode: string;
	messageGroupId: string;
	payload?: unknown;
	authToken?: string | undefined;
	callbackUrl?: string | undefined;
	createdAt?: string | undefined;
};

/**
 * Internal message representation with queue-specific metadata
 */
export interface QueueMessage {
	/** Queue-specific message ID (e.g., SQS MessageId) */
	brokerMessageId: string;
	/** Application message ID from payload */
	messageId: string;
	/** Receipt handle for ack/nack operations */
	receiptHandle: string;
	/** Parsed message pointer */
	pointer: MessagePointer;
	/** Approximate receive count */
	receiveCount: number;
	/** When the message was received by the consumer */
	receivedAt: Date;
	/** Batch ID for tracking batch+group FIFO */
	batchId: string;
	/** Queue identifier */
	queueId: string;
}

/**
 * Batch of messages from a single poll operation
 */
export interface MessageBatch {
	/** Unique batch ID */
	batchId: string;
	/** Messages in this batch */
	messages: QueueMessage[];
	/** Source queue identifier */
	queueId: string;
	/** When the batch was received */
	receivedAt: Date;
}

/**
 * Processing outcome - matches Java MediationResult
 */
export const ProcessingOutcome = {
	/** Message processed successfully - ACK */
	SUCCESS: 'SUCCESS',
	/** Configuration error (4xx) - ACK to prevent infinite retry */
	ERROR_CONFIG: 'ERROR_CONFIG',
	/** Processing error (5xx, timeout) - NACK for retry */
	ERROR_PROCESS: 'ERROR_PROCESS',
	/** Connection error - NACK for retry */
	ERROR_CONNECTION: 'ERROR_CONNECTION',
	/** Message deferred (ack=false response) - NACK with visibility */
	DEFERRED: 'DEFERRED',
	/** Batch+group already failed - NACK without processing */
	BATCH_FAILED: 'BATCH_FAILED',
} as const;

export type ProcessingOutcome = (typeof ProcessingOutcome)[keyof typeof ProcessingOutcome];

/**
 * Result of processing a message
 */
export interface ProcessingResult {
	outcome: ProcessingOutcome;
	/** Error message if failed */
	error?: string;
	/** HTTP status code from downstream */
	statusCode?: number;
	/** Processing duration in milliseconds */
	durationMs: number;
	/** Optional delay before retry (seconds) */
	delaySeconds?: number;
}
