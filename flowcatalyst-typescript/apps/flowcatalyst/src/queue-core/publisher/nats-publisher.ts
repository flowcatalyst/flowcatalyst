/**
 * NATS JetStream Queue Publisher
 *
 * Publishes messages to NATS JetStream subjects.
 * Subject is derived from a configured prefix + messageGroupId.
 */

import {
	connect,
	type NatsConnection,
	type JetStreamClient,
	headers,
} from "nats";
import type {
	QueuePublisher,
	PublishMessage,
	PublishResult,
} from "./queue-publisher.js";

/** NATS publisher configuration */
export interface NatsPublisherConfig {
	servers: string;
	connectionName?: string | undefined;
	subjectPrefix: string;
	username?: string | undefined;
	password?: string | undefined;
}

export async function createNatsPublisher(
	config: NatsPublisherConfig,
): Promise<QueuePublisher> {
	let nc: NatsConnection | null = null;
	let js: JetStreamClient | null = null;

	async function ensureConnected(): Promise<JetStreamClient> {
		if (js) return js;

		nc = await connect({
			servers: config.servers,
			name: config.connectionName ?? "flowcatalyst-publisher",
			...(config.username && config.password
				? { user: config.username, pass: config.password }
				: {}),
		});

		js = nc.jetstream();
		return js;
	}

	async function publish(message: PublishMessage): Promise<PublishResult> {
		try {
			const client = await ensureConnected();
			const subject = `${config.subjectPrefix}.${message.messageGroupId}`;

			const hdrs = headers();
			hdrs.set("FC-Message-Id", message.messageId);
			if (message.messageDeduplicationId) {
				hdrs.set("Nats-Msg-Id", message.messageDeduplicationId);
			}

			await client.publish(subject, new TextEncoder().encode(message.body), {
				headers: hdrs,
				msgID: message.messageDeduplicationId ?? message.messageId,
			});

			return { messageId: message.messageId, success: true };
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
		const results: PublishResult[] = [];
		for (const msg of messages) {
			results.push(await publish(msg));
		}
		return results;
	}

	return { publish, publishBatch };
}
