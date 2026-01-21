import type { Logger } from '@flowcatalyst/logging';
import type { QueueManagerService } from './queue-manager-service.js';
import { randomUUID } from 'node:crypto';

/**
 * Seed request parameters
 */
export interface SeedRequest {
	count: number;
	queue: string;
	endpoint: string;
	messageGroupMode: string;
}

/**
 * Seed result
 */
export interface SeedResult {
	messagesSent: number;
}

/**
 * Service for seeding test messages
 */
export class SeederService {
	private readonly queueManager: QueueManagerService;
	private readonly logger: Logger;

	// Predefined queue mappings
	private readonly queueMappings: Record<string, string> = {
		high: 'flow-catalyst-high-priority.fifo',
		medium: 'flow-catalyst-medium-priority.fifo',
		low: 'flow-catalyst-low-priority.fifo',
	};

	// Predefined endpoint mappings
	private readonly endpointMappings: Record<string, string> = {
		fast: 'http://localhost:8080/api/test/fast',
		slow: 'http://localhost:8080/api/test/slow',
		faulty: 'http://localhost:8080/api/test/faulty',
		fail: 'http://localhost:8080/api/test/fail',
		success: 'http://localhost:8080/api/test/success',
	};

	// Message group prefixes for different modes
	private readonly messageGroups = [
		'group-a',
		'group-b',
		'group-c',
		'group-d',
		'group-e',
		'group-f',
		'group-g',
		'group-h',
	];

	constructor(queueManager: QueueManagerService, logger: Logger) {
		this.queueManager = queueManager;
		this.logger = logger.child({ component: 'SeederService' });
	}

	/**
	 * Seed messages to queue
	 */
	async seedMessages(request: SeedRequest): Promise<SeedResult> {
		const { count, queue, endpoint, messageGroupMode } = request;

		this.logger.info({ count, queue, endpoint, messageGroupMode }, 'Seeding messages');

		// Check if embedded queue is available
		if (!this.queueManager.hasEmbeddedQueue()) {
			this.logger.warn('Embedded queue not available - seeding only works in EMBEDDED mode');
			return { messagesSent: 0 };
		}

		// Resolve queue
		const resolvedQueue = this.resolveQueue(queue);

		// Resolve endpoint
		const resolvedEndpoint = this.resolveEndpoint(endpoint);

		// Generate and send messages
		let messagesSent = 0;
		let duplicates = 0;
		let errors = 0;

		for (let i = 0; i < count; i++) {
			const messageGroupId = this.getMessageGroupId(messageGroupMode, i);
			const messageId = randomUUID();

			// Build the message payload that will be processed by the consumer
			const messagePayload = {
				messageId,
				poolCode: this.getPoolCodeFromQueue(resolvedQueue),
				messageGroupId,
				payload: {
					index: i,
					timestamp: new Date().toISOString(),
					testData: `Test message ${i + 1} of ${count}`,
				},
				callbackUrl: resolvedEndpoint,
				createdAt: new Date().toISOString(),
			};

			// Publish to embedded queue
			const result = this.queueManager.publishToEmbeddedQueue({
				messageId,
				messageGroupId,
				messageDeduplicationId: messageId, // Use messageId for deduplication
				payload: messagePayload,
			});

			if (result.success) {
				if (result.deduplicated) {
					duplicates++;
				} else {
					messagesSent++;
				}
				this.logger.debug({ messageId, queue: resolvedQueue }, 'Message published');
			} else {
				errors++;
				this.logger.error({ messageId, error: result.error }, 'Failed to publish message');
			}
		}

		this.logger.info(
			{ messagesSent, duplicates, errors, queue: resolvedQueue },
			'Messages seeded',
		);

		return { messagesSent };
	}

	/**
	 * Resolve queue name from shorthand or return as-is
	 */
	private resolveQueue(queue: string): string {
		if (queue === 'random') {
			const keys = Object.keys(this.queueMappings);
			if (keys.length === 0) return queue;
			const randomKey = keys[Math.floor(Math.random() * keys.length)] as string;
			return this.queueMappings[randomKey] ?? queue;
		}
		return this.queueMappings[queue] ?? queue;
	}

	/**
	 * Resolve endpoint from shorthand or return as-is
	 */
	private resolveEndpoint(endpoint: string): string {
		if (endpoint === 'random') {
			const keys = Object.keys(this.endpointMappings);
			if (keys.length === 0) return endpoint;
			const randomKey = keys[Math.floor(Math.random() * keys.length)] as string;
			return this.endpointMappings[randomKey] ?? endpoint;
		}
		return this.endpointMappings[endpoint] ?? endpoint;
	}

	/**
	 * Get message group ID based on mode
	 */
	private getMessageGroupId(mode: string, index: number): string {
		switch (mode) {
			case 'unique':
				return randomUUID();
			case 'single':
				return 'single-group';
			case '1of8':
			default:
				return this.messageGroups[index % this.messageGroups.length] ?? 'group-a';
		}
	}

	/**
	 * Get pool code from queue name
	 */
	private getPoolCodeFromQueue(queue: string): string {
		if (queue.includes('high')) return 'POOL-HIGH';
		if (queue.includes('medium')) return 'POOL-MEDIUM';
		if (queue.includes('low')) return 'POOL-LOW';
		return 'POOL-DEFAULT';
	}
}
