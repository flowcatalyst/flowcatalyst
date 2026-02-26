/**
 * Embedded Queue Publisher Adapter
 *
 * Wraps a publish function (typically backed by the message-router's
 * EmbeddedQueuePublisher) to implement the broker-agnostic QueuePublisher interface.
 */

import type {
	QueuePublisher,
	PublishMessage,
	PublishResult,
} from "./queue-publisher.js";

/** Callback signature matching QueueManagerService.publishToEmbeddedQueue() */
export type EmbeddedPublishFn = (message: {
	messageId: string;
	messageGroupId: string;
	messageDeduplicationId?: string | undefined;
	payload: unknown;
}) => {
	success: boolean;
	error?: string | undefined;
	deduplicated?: boolean | undefined;
};

export function createEmbeddedPublisher(
	publishFn: EmbeddedPublishFn,
): QueuePublisher {
	async function publish(message: PublishMessage): Promise<PublishResult> {
		try {
			const result = publishFn({
				messageId: message.messageId,
				messageGroupId: message.messageGroupId,
				messageDeduplicationId: message.messageDeduplicationId,
				payload: JSON.parse(message.body),
			});

			return {
				messageId: message.messageId,
				success: result.success,
				error: result.error,
			};
		} catch (error) {
			return {
				messageId: message.messageId,
				success: false,
				error: error instanceof Error ? error.message : String(error),
			};
		}
	}

	async function publishBatch(
		messages: PublishMessage[],
	): Promise<PublishResult[]> {
		return Promise.all(messages.map(publish));
	}

	return { publish, publishBatch };
}
