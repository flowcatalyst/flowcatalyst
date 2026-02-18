/**
 * ActiveMQ Queue Publisher
 *
 * Publishes messages to ActiveMQ using STOMP protocol.
 * Sets JMSXGroupID header for FIFO ordering per message group.
 */

import stompit from 'stompit';
import type { QueuePublisher, PublishMessage, PublishResult } from './queue-publisher.js';

/** ActiveMQ publisher configuration */
export interface ActiveMqPublisherConfig {
  host: string;
  port: number;
  username: string;
  password: string;
  destination: string;
}

export function createActiveMqPublisher(config: ActiveMqPublisherConfig): QueuePublisher {
  function sendMessage(message: PublishMessage): Promise<PublishResult> {
    return new Promise((resolve) => {
      const connectOptions = {
        host: config.host,
        port: config.port,
        connectHeaders: {
          host: '/',
          login: config.username,
          passcode: config.password,
          'heart-beat': '0,0',
        },
      };

      stompit.connect(connectOptions, (error, client) => {
        if (error) {
          resolve({
            messageId: message.messageId,
            success: false,
            error: error instanceof Error ? error.message : String(error),
          });
          return;
        }

        const sendHeaders: Record<string, string> = {
          destination: config.destination,
          'content-type': 'application/json',
          'message-id': message.messageId,
          JMSXGroupID: message.messageGroupId,
        };

        if (message.messageDeduplicationId) {
          sendHeaders['x-deduplication-id'] = message.messageDeduplicationId;
        }

        const frame = client.send(sendHeaders);
        frame.write(message.body);
        frame.end();

        client.disconnect((disconnectError) => {
          if (disconnectError) {
            resolve({
              messageId: message.messageId,
              success: false,
              error:
                disconnectError instanceof Error
                  ? disconnectError.message
                  : String(disconnectError),
            });
          } else {
            resolve({ messageId: message.messageId, success: true });
          }
        });
      });
    });
  }

  async function publish(message: PublishMessage): Promise<PublishResult> {
    return sendMessage(message);
  }

  async function publishBatch(messages: PublishMessage[]): Promise<PublishResult[]> {
    const results: PublishResult[] = [];
    for (const msg of messages) {
      results.push(await sendMessage(msg));
    }
    return results;
  }

  return { publish, publishBatch };
}
