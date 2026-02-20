/**
 * Queue Publisher Interface
 *
 * Broker-agnostic publish interface — the counterpart to QueueConsumer.
 * All queue implementations (SQS, NATS, ActiveMQ, Embedded) implement this
 * interface to provide a consistent publishing contract.
 */

/**
 * Message to publish to a queue.
 */
export interface PublishMessage {
	messageId: string;
	messageGroupId: string;
	messageDeduplicationId?: string | undefined;
	body: string;
}

/**
 * Result of publishing a single message.
 */
export interface PublishResult {
	messageId: string;
	success: boolean;
	error?: string | undefined;
}

/**
 * Queue publisher interface — all broker implementations must satisfy this contract.
 */
export interface QueuePublisher {
	/** Publish a batch of messages. Returns results per message. */
	publishBatch(messages: PublishMessage[]): Promise<PublishResult[]>;

	/** Publish a single message. */
	publish(message: PublishMessage): Promise<PublishResult>;
}
