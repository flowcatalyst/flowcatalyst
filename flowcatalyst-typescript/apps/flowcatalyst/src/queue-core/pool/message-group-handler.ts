import type { Logger } from '@flowcatalyst/logging';
import type { QueueMessage } from '@flowcatalyst/shared-types';
import type { MessageCallback } from './process-pool.js';

/**
 * Message with its callback
 */
interface QueuedMessage {
  message: QueueMessage;
  callback: MessageCallback;
}

/**
 * Handler for a single message group - processes messages sequentially (FIFO)
 * Matches Java's per-message-group virtual thread pattern
 *
 * Supports two-tier priority: high-priority messages are dequeued before
 * regular messages within the same group, matching Java ProcessPoolImpl.
 */
export class MessageGroupHandler {
  private readonly messageGroupId: string;
  private readonly processor: (message: QueueMessage, callback: MessageCallback) => Promise<void>;
  private readonly onCleanup: () => void;
  private readonly logger: Logger;

  private readonly highPriorityQueue: QueuedMessage[] = [];
  private readonly regularQueue: QueuedMessage[] = [];
  private processing = false;
  private idleTimeoutHandle: ReturnType<typeof setTimeout> | null = null;

  // Idle timeout - cleanup after 5 minutes of inactivity
  private static readonly IDLE_TIMEOUT_MS = 5 * 60 * 1000;

  constructor(
    messageGroupId: string,
    processor: (message: QueueMessage, callback: MessageCallback) => Promise<void>,
    onCleanup: () => void,
    logger: Logger,
  ) {
    this.messageGroupId = messageGroupId;
    this.processor = processor;
    this.onCleanup = onCleanup;
    this.logger = logger.child({ messageGroupId });
  }

  /**
   * Enqueue a message for processing
   * Routes to high-priority or regular queue based on message flag
   */
  enqueue(message: QueueMessage, callback: MessageCallback): void {
    if (message.pointer.highPriority) {
      this.highPriorityQueue.push({ message, callback });
    } else {
      this.regularQueue.push({ message, callback });
    }
    this.clearIdleTimeout();
    this.processNext();
  }

  /**
   * Process messages sequentially, high-priority first
   */
  private async processNext(): Promise<void> {
    if (this.processing) {
      return; // Already processing
    }

    // Dequeue: high-priority first, then regular
    const item = this.highPriorityQueue.shift() ?? this.regularQueue.shift();
    if (!item) {
      // Queue empty - start idle timeout
      this.startIdleTimeout();
      return;
    }

    this.processing = true;

    try {
      await this.processor(item.message, item.callback);
    } catch (error) {
      this.logger.error(
        { err: error, messageId: item.message.messageId },
        'Message group handler error',
      );
    } finally {
      this.processing = false;
      // Process next message (if any)
      setImmediate(() => this.processNext());
    }
  }

  /**
   * Start idle timeout for cleanup
   */
  private startIdleTimeout(): void {
    this.idleTimeoutHandle = setTimeout(() => {
      if (
        this.highPriorityQueue.length === 0 &&
        this.regularQueue.length === 0 &&
        !this.processing
      ) {
        this.logger.debug('Message group handler idle timeout - cleaning up');
        this.onCleanup();
      }
    }, MessageGroupHandler.IDLE_TIMEOUT_MS);
  }

  /**
   * Clear idle timeout
   */
  private clearIdleTimeout(): void {
    if (this.idleTimeoutHandle) {
      clearTimeout(this.idleTimeoutHandle);
      this.idleTimeoutHandle = null;
    }
  }

  /**
   * Get queue size (both queues combined)
   */
  getQueueSize(): number {
    return this.highPriorityQueue.length + this.regularQueue.length;
  }

  /**
   * Check if currently processing
   */
  isProcessing(): boolean {
    return this.processing;
  }
}
