import { request, Agent } from 'undici';
import type { Dispatcher } from 'undici';
import type { Logger } from '@flowcatalyst/logging';
import type { MediationResponse, ProcessingResult, QueueMessage } from '@flowcatalyst/shared-types';
import type { CircuitBreakerManager } from './circuit-breaker.js';

/**
 * HTTP Mediator configuration
 */
export interface HttpMediatorConfig {
	/** Default callback URL */
	callbackUrl: string;
	/** Use HTTP/2 (true) or HTTP/1.1 (false) */
	useHttp2: boolean;
	/** Connection timeout in milliseconds */
	connectTimeoutMs: number;
	/** Headers timeout in milliseconds */
	headersTimeoutMs: number;
	/** Body/request timeout in milliseconds */
	bodyTimeoutMs: number;
	/** Number of retries */
	retries: number;
	/** Initial retry delay in milliseconds */
	retryDelayMs: number;
}

/**
 * HTTP Mediator - calls downstream services
 *
 * - HTTP/2 for production (single multiplexed connection)
 * - HTTP/1.1 for local dev
 * - Separate connection timeout vs request timeout
 * - Circuit breaker integration
 * - Exponential backoff retry
 */
export class HttpMediator {
	private readonly config: HttpMediatorConfig;
	private readonly logger: Logger;
	private readonly circuitBreakers: CircuitBreakerManager;
	private readonly agent: Agent;

	constructor(
		config: HttpMediatorConfig,
		circuitBreakers: CircuitBreakerManager,
		logger: Logger,
	) {
		this.config = config;
		this.circuitBreakers = circuitBreakers;
		this.logger = logger.child({ component: 'HttpMediator' });

		this.agent = new Agent({
			connect: {
				timeout: config.connectTimeoutMs,
			},
			headersTimeout: config.headersTimeoutMs,
			bodyTimeout: config.bodyTimeoutMs,
			allowH2: config.useHttp2,
		});

		this.logger.info(
			{
				http2: config.useHttp2,
				connectTimeoutMs: config.connectTimeoutMs,
				bodyTimeoutMs: config.bodyTimeoutMs,
			},
			'HTTP mediator initialized',
		);
	}

	/**
	 * Process a message by calling the downstream service
	 */
	async process(message: QueueMessage): Promise<ProcessingResult> {
		const startTime = Date.now();
		const callbackUrl = message.pointer.callbackUrl || this.config.callbackUrl;

		const circuitBreaker = this.circuitBreakers.getOrCreate(callbackUrl);

		try {
			const result = await circuitBreaker.execute(async () => {
				return this.executeWithRetry(message, callbackUrl);
			});
			return result;
		} catch (error) {
			const durationMs = Date.now() - startTime;

			if (error instanceof Error && error.message.includes('Circuit breaker is open')) {
				this.logger.warn(
					{ callbackUrl, messageId: message.messageId },
					'Circuit breaker open - request rejected',
				);
				return {
					outcome: 'ERROR_PROCESS',
					error: 'Circuit breaker open',
					durationMs,
				};
			}

			this.logger.error(
				{ err: error, callbackUrl, messageId: message.messageId },
				'Mediation failed',
			);
			return {
				outcome: 'ERROR_PROCESS',
				error: error instanceof Error ? error.message : 'Unknown error',
				durationMs,
			};
		}
	}

	/**
	 * Execute HTTP request with retry logic
	 */
	private async executeWithRetry(
		message: QueueMessage,
		callbackUrl: string,
	): Promise<ProcessingResult> {
		let lastError: Error | null = null;
		const startTime = Date.now();

		for (let attempt = 0; attempt <= this.config.retries; attempt++) {
			try {
				if (attempt > 0) {
					const delay = this.config.retryDelayMs * Math.pow(2, attempt - 1);
					this.logger.debug(
						{ attempt, delay, messageId: message.messageId },
						'Retrying request',
					);
					await sleep(delay);
				}

				const result = await this.executeRequest(message, callbackUrl);

				// Don't retry on success or client errors
				if (result.outcome === 'SUCCESS' || result.outcome === 'ERROR_CONFIG' || result.outcome === 'DEFERRED') {
					return result;
				}

				lastError = new Error(result.error || 'Server error');
			} catch (error) {
				lastError = error instanceof Error ? error : new Error(String(error));

				if (this.isConnectionTimeout(lastError)) {
					this.logger.warn(
						{ err: lastError, attempt, messageId: message.messageId },
						'Connection timeout',
					);
				}
			}
		}

		const durationMs = Date.now() - startTime;
		return {
			outcome: 'ERROR_PROCESS',
			error: lastError?.message || 'Max retries exceeded',
			durationMs,
		};
	}

	/**
	 * Execute a single HTTP request
	 */
	private async executeRequest(
		message: QueueMessage,
		callbackUrl: string,
	): Promise<ProcessingResult> {
		const startTime = Date.now();

		try {
			const headers: Record<string, string> = {
				'Content-Type': 'application/json',
				'Accept': 'application/json',
				'X-Message-Id': message.messageId,
				'X-Broker-Message-Id': message.brokerMessageId,
				'X-Pool-Code': message.pointer.poolCode,
			};

			if (message.pointer.authToken) {
				headers['Authorization'] = `Bearer ${message.pointer.authToken}`;
			}

			const body = JSON.stringify(message.pointer.payload);

			const response = await request(callbackUrl, {
				method: 'POST',
				headers,
				body,
				dispatcher: this.agent,
			});

			const durationMs = Date.now() - startTime;
			const statusCode = response.statusCode;

			if (statusCode >= 200 && statusCode < 300) {
				return this.handleSuccessResponse(response, statusCode, durationMs);
			}

			if (statusCode >= 400 && statusCode < 500) {
				const errorBody = await this.readResponseBody(response);
				this.logger.warn(
					{ statusCode, callbackUrl, error: errorBody },
					'Client error from downstream',
				);
				return {
					outcome: 'ERROR_CONFIG',
					statusCode,
					error: `HTTP ${statusCode}: ${errorBody || 'Client Error'}`,
					durationMs,
				};
			}

			const errorBody = await this.readResponseBody(response);
			this.logger.warn(
				{ statusCode, callbackUrl, error: errorBody },
				'Server error from downstream',
			);
			return {
				outcome: 'ERROR_PROCESS',
				statusCode,
				error: `HTTP ${statusCode}: ${errorBody || 'Server Error'}`,
				durationMs,
			};
		} catch (error) {
			const durationMs = Date.now() - startTime;
			const errorMessage = error instanceof Error ? error.message : String(error);

			if (this.isConnectionTimeout(error)) {
				return {
					outcome: 'ERROR_PROCESS',
					error: `Connection timeout after ${this.config.connectTimeoutMs}ms`,
					durationMs,
				};
			}

			if (this.isBodyTimeout(error)) {
				return {
					outcome: 'ERROR_PROCESS',
					error: `Request timeout after ${this.config.bodyTimeoutMs}ms`,
					durationMs,
				};
			}

			return {
				outcome: 'ERROR_PROCESS',
				error: errorMessage,
				durationMs,
			};
		}
	}

	private async handleSuccessResponse(
		response: Dispatcher.ResponseData,
		statusCode: number,
		durationMs: number,
	): Promise<ProcessingResult> {
		try {
			const bodyText = await this.readResponseBody(response);

			if (!bodyText) {
				return { outcome: 'SUCCESS', statusCode, durationMs };
			}

			const body = JSON.parse(bodyText) as MediationResponse;

			if (typeof body.ack === 'boolean') {
				if (body.ack) {
					return { outcome: 'SUCCESS', statusCode, durationMs };
				} else {
					return {
						outcome: 'DEFERRED',
						statusCode,
						error: body.message || 'Message deferred',
						durationMs,
					};
				}
			}

			return { outcome: 'SUCCESS', statusCode, durationMs };
		} catch {
			return { outcome: 'SUCCESS', statusCode, durationMs };
		}
	}

	private async readResponseBody(response: Dispatcher.ResponseData): Promise<string> {
		try {
			const chunks: Buffer[] = [];
			for await (const chunk of response.body) {
				chunks.push(Buffer.from(chunk));
			}
			return Buffer.concat(chunks).toString('utf-8');
		} catch {
			return '';
		}
	}

	private isConnectionTimeout(error: unknown): boolean {
		if (!(error instanceof Error)) return false;
		const message = error.message.toLowerCase();
		return (
			message.includes('connect timeout') ||
			message.includes('connection timeout') ||
			message.includes('etimedout') ||
			message.includes('econnrefused') ||
			message.includes('enotfound') ||
			error.name === 'ConnectTimeoutError'
		);
	}

	private isBodyTimeout(error: unknown): boolean {
		if (!(error instanceof Error)) return false;
		const message = error.message.toLowerCase();
		return (
			message.includes('body timeout') ||
			message.includes('headers timeout') ||
			message.includes('request timeout') ||
			error.name === 'BodyTimeoutError' ||
			error.name === 'HeadersTimeoutError'
		);
	}

	async close(): Promise<void> {
		await this.agent.close();
		this.logger.info('HTTP mediator closed');
	}
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
