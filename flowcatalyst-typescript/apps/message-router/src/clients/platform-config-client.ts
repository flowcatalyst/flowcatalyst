import type { Logger } from '@flowcatalyst/logging';
import type { LocalConfigResponse, ProcessingPoolDto, QueueConfigDto } from '@flowcatalyst/shared-types';

/**
 * Platform configuration client options
 */
export interface PlatformConfigClientOptions {
	/** Platform API base URL */
	baseUrl: string;
	/** Optional API key for authentication */
	apiKey?: string | undefined;
	/** Connection timeout in milliseconds */
	connectTimeoutMs: number;
	/** Read timeout in milliseconds */
	readTimeoutMs: number;
	/** Max retry attempts */
	maxAttempts: number;
	/** Delay between retries in milliseconds */
	retryDelayMs: number;
}

/**
 * Default configuration options
 */
export const defaultPlatformConfigClientOptions: PlatformConfigClientOptions = {
	baseUrl: 'http://localhost:8080',
	connectTimeoutMs: 3000,
	readTimeoutMs: 5000,
	maxAttempts: 12,
	retryDelayMs: 5000,
};

/**
 * Router configuration returned by platform
 */
export interface MessageRouterConfig {
	queues: QueueConfigDto[];
	connections: number;
	processingPools: ProcessingPoolDto[];
}

/**
 * Platform Config Client
 * Fetches router configuration from the platform API
 * Matches Java MessageRouterConfigClient behavior
 */
export class PlatformConfigClient {
	private readonly options: PlatformConfigClientOptions;
	private readonly logger: Logger;
	private readonly configEndpoint: string;

	constructor(options: Partial<PlatformConfigClientOptions>, logger: Logger) {
		this.options = { ...defaultPlatformConfigClientOptions, ...options };
		this.logger = logger.child({ component: 'PlatformConfigClient' });
		this.configEndpoint = `${this.options.baseUrl}/api/router/config`;
	}

	/**
	 * Fetch router configuration from platform
	 * Retries up to maxAttempts times with retryDelayMs between attempts
	 */
	async fetchConfig(): Promise<MessageRouterConfig | null> {
		let attempts = 0;

		while (attempts < this.options.maxAttempts) {
			attempts++;

			try {
				const config = await this.doFetch();
				this.logger.info(
					{
						queues: config.queues.length,
						pools: config.processingPools.length,
						connections: config.connections,
					},
					'Successfully fetched router configuration',
				);
				return config;
			} catch (error) {
				const errorMessage = error instanceof Error ? error.message : String(error);

				if (attempts >= this.options.maxAttempts) {
					this.logger.error(
						{ attempts, error: errorMessage },
						'Failed to fetch router configuration after max attempts',
					);
					return null;
				}

				this.logger.warn(
					{ attempts, maxAttempts: this.options.maxAttempts, error: errorMessage },
					'Failed to fetch router configuration, retrying...',
				);

				await sleep(this.options.retryDelayMs);
			}
		}

		return null;
	}

	/**
	 * Fetch configuration once (no retry)
	 */
	async fetchConfigOnce(): Promise<MessageRouterConfig> {
		return this.doFetch();
	}

	/**
	 * Execute the fetch request
	 */
	private async doFetch(): Promise<MessageRouterConfig> {
		const controller = new AbortController();
		const timeoutId = setTimeout(
			() => controller.abort(),
			this.options.connectTimeoutMs + this.options.readTimeoutMs,
		);

		try {
			const headers: Record<string, string> = {
				'Accept': 'application/json',
				'Content-Type': 'application/json',
			};

			// Add API key if configured
			if (this.options.apiKey) {
				headers['X-API-Key'] = this.options.apiKey;
			}

			const response = await fetch(this.configEndpoint, {
				method: 'GET',
				headers,
				signal: controller.signal,
			});

			if (!response.ok) {
				throw new Error(`HTTP ${response.status}: ${response.statusText}`);
			}

			const data = (await response.json()) as MessageRouterConfig;

			// Validate response structure
			if (!data.queues || !Array.isArray(data.queues)) {
				throw new Error('Invalid config: missing queues array');
			}
			if (!data.processingPools || !Array.isArray(data.processingPools)) {
				throw new Error('Invalid config: missing processingPools array');
			}
			if (typeof data.connections !== 'number') {
				throw new Error('Invalid config: missing connections number');
			}

			return data;
		} catch (error) {
			if (error instanceof Error && error.name === 'AbortError') {
				throw new Error(`Request timeout after ${this.options.connectTimeoutMs + this.options.readTimeoutMs}ms`);
			}
			throw error;
		} finally {
			clearTimeout(timeoutId);
		}
	}

	/**
	 * Check if platform is reachable
	 */
	async healthCheck(): Promise<boolean> {
		try {
			const controller = new AbortController();
			const timeoutId = setTimeout(() => controller.abort(), this.options.connectTimeoutMs);

			try {
				const response = await fetch(`${this.options.baseUrl}/health/ready`, {
					method: 'GET',
					signal: controller.signal,
				});
				return response.ok;
			} finally {
				clearTimeout(timeoutId);
			}
		} catch {
			return false;
		}
	}
}

/**
 * Configuration sync service
 * Handles periodic sync of router configuration
 */
export class ConfigSyncService {
	private readonly client: PlatformConfigClient;
	private readonly logger: Logger;
	private readonly syncIntervalMs: number;
	private readonly onConfigUpdate: (config: MessageRouterConfig) => Promise<void>;

	private running = false;
	private syncTask: Promise<void> | null = null;
	private lastSyncTime = 0;
	private lastConfig: MessageRouterConfig | null = null;

	constructor(
		client: PlatformConfigClient,
		syncIntervalMs: number,
		onConfigUpdate: (config: MessageRouterConfig) => Promise<void>,
		logger: Logger,
	) {
		this.client = client;
		this.syncIntervalMs = syncIntervalMs;
		this.onConfigUpdate = onConfigUpdate;
		this.logger = logger.child({ component: 'ConfigSyncService' });
	}

	/**
	 * Start periodic configuration sync
	 */
	async start(): Promise<boolean> {
		this.logger.info('Starting configuration sync service');

		// Initial sync with retry
		const initialConfig = await this.client.fetchConfig();
		if (!initialConfig) {
			this.logger.error('Failed initial configuration sync');
			return false;
		}

		this.lastConfig = initialConfig;
		this.lastSyncTime = Date.now();

		// Notify handler of initial config
		await this.onConfigUpdate(initialConfig);

		// Start periodic sync
		this.running = true;
		this.syncTask = this.syncLoop();

		return true;
	}

	/**
	 * Stop configuration sync
	 */
	async stop(): Promise<void> {
		this.logger.info('Stopping configuration sync service');
		this.running = false;

		if (this.syncTask) {
			await this.syncTask;
			this.syncTask = null;
		}
	}

	/**
	 * Force an immediate sync
	 */
	async syncNow(): Promise<boolean> {
		return this.doSync();
	}

	/**
	 * Get last synced configuration
	 */
	getLastConfig(): MessageRouterConfig | null {
		return this.lastConfig;
	}

	/**
	 * Get time since last successful sync
	 */
	getTimeSinceLastSync(): number {
		return this.lastSyncTime > 0 ? Date.now() - this.lastSyncTime : -1;
	}

	/**
	 * Periodic sync loop
	 */
	private async syncLoop(): Promise<void> {
		// Initial delay (2 seconds as per Java config)
		await sleep(2000);

		while (this.running) {
			await this.doSync();
			await sleep(this.syncIntervalMs);
		}
	}

	/**
	 * Execute a single sync
	 */
	private async doSync(): Promise<boolean> {
		try {
			const config = await this.client.fetchConfigOnce();

			// Check if config changed
			const configChanged = !this.configEquals(config, this.lastConfig);

			this.lastConfig = config;
			this.lastSyncTime = Date.now();

			if (configChanged) {
				this.logger.info('Configuration changed, applying updates');
				await this.onConfigUpdate(config);
			} else {
				this.logger.debug('Configuration unchanged');
			}

			return true;
		} catch (error) {
			this.logger.error({ err: error }, 'Configuration sync failed');
			return false;
		}
	}

	/**
	 * Compare two configurations for equality
	 */
	private configEquals(
		a: MessageRouterConfig | null,
		b: MessageRouterConfig | null,
	): boolean {
		if (a === null || b === null) return a === b;
		if (a.connections !== b.connections) return false;
		if (a.queues.length !== b.queues.length) return false;
		if (a.processingPools.length !== b.processingPools.length) return false;

		// Deep compare queues
		for (let i = 0; i < a.queues.length; i++) {
			const qa = a.queues[i];
			const qb = b.queues[i];
			if (!qa || !qb) return false;
			if (qa.queueUri !== qb.queueUri) return false;
			if (qa.queueName !== qb.queueName) return false;
			if (qa.connections !== qb.connections) return false;
		}

		// Deep compare pools
		for (let i = 0; i < a.processingPools.length; i++) {
			const pa = a.processingPools[i];
			const pb = b.processingPools[i];
			if (!pa || !pb) return false;
			if (pa.code !== pb.code) return false;
			if (pa.concurrency !== pb.concurrency) return false;
			if (pa.rateLimitPerMinute !== pb.rateLimitPerMinute) return false;
		}

		return true;
	}
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
