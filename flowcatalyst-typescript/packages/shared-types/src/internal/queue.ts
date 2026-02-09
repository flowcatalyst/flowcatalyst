/**
 * Supported queue types
 */
export const QueueType = {
  SQS: 'SQS',
  NATS: 'NATS',
  EMBEDDED: 'EMBEDDED',
} as const;

export type QueueType = (typeof QueueType)[keyof typeof QueueType];

/**
 * Base queue configuration - internal representation
 * Matches Java QueueConfig
 */
export interface BaseQueueConfig {
  /** Queue URI (unique identifier) */
  queueUri: string;
  /** Human-readable queue name */
  queueName: string | null;
  /** Number of parallel polling connections (null = use global) */
  connections: number | null;
}

/**
 * SQS-specific configuration
 */
export interface SqsQueueConfig extends BaseQueueConfig {
  type: 'SQS';
  /** AWS region */
  region: string;
  /** Long poll wait time in seconds */
  waitTimeSeconds: number;
  /** Max messages per poll */
  maxMessages: number;
  /** Visibility timeout in seconds */
  visibilityTimeout: number;
}

/**
 * NATS-specific configuration
 */
export interface NatsQueueConfig extends BaseQueueConfig {
  type: 'NATS';
  /** NATS server URLs */
  servers: string[];
  /** Stream name */
  stream: string;
  /** Consumer name */
  consumer: string;
  /** Subject to subscribe to */
  subject: string;
}

/**
 * Embedded queue configuration (for local dev)
 */
export interface EmbeddedQueueConfig extends BaseQueueConfig {
  type: 'EMBEDDED';
  /** SQLite database path */
  dbPath: string;
  /** Visibility timeout in seconds */
  visibilityTimeout: number;
}

/**
 * Union of all queue configurations
 */
export type QueueConfig = SqsQueueConfig | NatsQueueConfig | EmbeddedQueueConfig;

/**
 * Internal queue statistics tracking
 */
export interface QueueStatsInternal {
  /** Queue name */
  name: string;
  /** Queue URI */
  queueUri: string;
  /** Total messages received */
  totalMessages: number;
  /** Total successfully consumed */
  totalConsumed: number;
  /** Total failed */
  totalFailed: number;
  /** Total deferred */
  totalDeferred: number;
  /** Current queue size (from SQS attributes) */
  currentSize: number;
  /** Pending messages */
  pendingMessages: number;
  /** Messages not visible */
  messagesNotVisible: number;
  /** Windowed stats */
  windowedStats: {
    messages5min: number;
    consumed5min: number;
    failed5min: number;
    messages30min: number;
    consumed30min: number;
    failed30min: number;
  };
}

/**
 * Message callback interface for ack/nack operations
 */
export interface MessageCallback {
  /** Acknowledge the message (delete from queue) */
  ack(): Promise<void>;
  /** Negative acknowledge (change visibility for retry) */
  nack(visibilityTimeoutSeconds?: number): Promise<void>;
  /** Update the receipt handle (for redelivered messages) */
  updateReceiptHandle(newHandle: string): void;
}
