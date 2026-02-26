/**
 * SQS Queue Publisher
 *
 * Publishes messages to an SQS FIFO queue using SendMessageBatch.
 * Batches in groups of 10 (SQS limit per batch request).
 */

import {
	SQSClient,
	SendMessageBatchCommand,
	type SendMessageBatchRequestEntry,
} from "@aws-sdk/client-sqs";
import type {
	QueuePublisher,
	PublishMessage,
	PublishResult,
} from "./queue-publisher.js";

/** SQS publisher configuration */
export interface SqsPublisherConfig {
	queueUrl: string;
	region: string;
	endpoint?: string | undefined;
}

/** SQS batch size limit */
const SQS_BATCH_LIMIT = 10;

export function createSqsPublisher(config: SqsPublisherConfig): QueuePublisher {
	const client = new SQSClient({
		region: config.region,
		...(config.endpoint ? { endpoint: config.endpoint } : {}),
	});

	async function publish(message: PublishMessage): Promise<PublishResult> {
		const results = await publishBatch([message]);
		return results[0]!;
	}

	async function publishBatch(
		messages: PublishMessage[],
	): Promise<PublishResult[]> {
		if (messages.length === 0) return [];

		const allResults: PublishResult[] = [];

		// Split into chunks of 10 (SQS batch limit)
		for (let i = 0; i < messages.length; i += SQS_BATCH_LIMIT) {
			const chunk = messages.slice(i, i + SQS_BATCH_LIMIT);
			const chunkResults = await sendBatch(chunk);
			allResults.push(...chunkResults);
		}

		return allResults;
	}

	async function sendBatch(
		messages: PublishMessage[],
	): Promise<PublishResult[]> {
		const entries: SendMessageBatchRequestEntry[] = messages.map((msg, idx) => {
			const entry: SendMessageBatchRequestEntry = {
				Id: String(idx),
				MessageBody: msg.body,
				MessageGroupId: msg.messageGroupId,
			};
			if (msg.messageDeduplicationId) {
				entry.MessageDeduplicationId = msg.messageDeduplicationId;
			}
			return entry;
		});

		try {
			const response = await client.send(
				new SendMessageBatchCommand({
					QueueUrl: config.queueUrl,
					Entries: entries,
				}),
			);

			// Build a results map: entry Id → success/failure
			const resultMap = new Map<string, PublishResult>();

			for (const s of response.Successful ?? []) {
				const idx = Number(s.Id);
				const msg = messages[idx]!;
				resultMap.set(s.Id!, { messageId: msg.messageId, success: true });
			}

			for (const f of response.Failed ?? []) {
				const idx = Number(f.Id);
				const msg = messages[idx]!;
				resultMap.set(f.Id!, {
					messageId: msg.messageId,
					success: false,
					error: `${f.Code}: ${f.Message}`,
				});
			}

			// Return in original order
			return messages.map((msg, idx) => {
				return (
					resultMap.get(String(idx)) ?? {
						messageId: msg.messageId,
						success: false,
						error: "No response from SQS",
					}
				);
			});
		} catch (error) {
			// Entire batch failed
			const errorMsg = error instanceof Error ? error.message : String(error);
			return messages.map((msg) => ({
				messageId: msg.messageId,
				success: false,
				error: errorMsg,
			}));
		}
	}

	return { publish, publishBatch };
}
