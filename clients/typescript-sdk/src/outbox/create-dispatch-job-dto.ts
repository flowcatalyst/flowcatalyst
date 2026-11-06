/**
 * DTO for creating a dispatch job in the outbox.
 *
 * Uses an immutable builder pattern - all `with*()` methods return a new instance.
 *
 * @example
 * ```typescript
 * const job = CreateDispatchJobDto
 *   .create('order-service', 'shop:orders:order:process', 'https://api.example.com/webhook', '{"orderId":"123"}', 'pool-1')
 *   .withCorrelationId('corr-789')
 *   .withTimeoutSeconds(60);
 * ```
 *
 * The payload is an item of `POST /api/dispatch-jobs/batch`. Fields the
 * platform does not honour on that route are kept for source compatibility but
 * are NOT sent: `headers`, `retryStrategy`, `scheduledFor` and `expiresAt` (and
 * their withers). Setting them has no effect.
 */
import { assertQualifiedCode } from "./qualified-code.js";

/**
 * Ordering behavior within a message group.
 * - IMMEDIATE: no ordering, jobs dispatch concurrently (platform default)
 * - NEXT_ON_ERROR: FIFO per message group; a failed job is retried later but the group moves on
 * - BLOCK_ON_ERROR: strict FIFO per message group; a failed job blocks the group until resolved
 */
export type DispatchMode = "IMMEDIATE" | "NEXT_ON_ERROR" | "BLOCK_ON_ERROR";

export class CreateDispatchJobDto {
	readonly source: string;
	readonly code: string;
	readonly targetUrl: string;
	readonly payload: string;
	readonly dispatchPoolId: string;
	readonly subject: string | null;
	readonly correlationId: string | null;
	readonly eventId: string | null;
	/** Metadata pairs in the order they were added; sent in this order. */
	readonly metadataPairs: ReadonlyArray<readonly [string, string]>;
	/** @deprecated Not sent: the platform does not take per-job headers. */
	readonly headers: Record<string, string>;
	readonly payloadContentType: string;
	readonly dataOnly: boolean;
	readonly messageGroup: string | null;
	readonly mode: DispatchMode | null;
	readonly sequence: number | null;
	readonly timeoutSeconds: number;
	readonly maxRetries: number;
	/** @deprecated Not sent: the batch route does not take one. */
	readonly retryStrategy: string | null;
	/** @deprecated Not sent: the batch route does not take one. */
	readonly scheduledFor: Date | null;
	/** @deprecated Not sent: the batch route does not take one. */
	readonly expiresAt: Date | null;
	readonly idempotencyKey: string | null;
	readonly externalId: string | null;
	readonly connectionId: string | null;
	readonly queue: string | null;

	private constructor(params: {
		source: string;
		code: string;
		targetUrl: string;
		payload: string;
		dispatchPoolId: string;
		subject?: string | null;
		correlationId?: string | null;
		eventId?: string | null;
		metadataPairs?: ReadonlyArray<readonly [string, string]>;
		headers?: Record<string, string>;
		payloadContentType?: string;
		dataOnly?: boolean;
		messageGroup?: string | null;
		mode?: DispatchMode | null;
		sequence?: number | null;
		timeoutSeconds?: number;
		maxRetries?: number;
		retryStrategy?: string | null;
		scheduledFor?: Date | null;
		expiresAt?: Date | null;
		idempotencyKey?: string | null;
		externalId?: string | null;
		connectionId?: string | null;
		queue?: string | null;
	}) {
		this.source = params.source;
		this.code = params.code;
		this.targetUrl = params.targetUrl;
		this.payload = params.payload;
		this.dispatchPoolId = params.dispatchPoolId;
		this.subject = params.subject ?? null;
		this.correlationId = params.correlationId ?? null;
		this.eventId = params.eventId ?? null;
		this.metadataPairs = params.metadataPairs ?? [];
		this.headers = params.headers ?? {};
		this.payloadContentType = params.payloadContentType ?? "application/json";
		this.dataOnly = params.dataOnly ?? true;
		this.messageGroup = params.messageGroup ?? null;
		this.mode = params.mode ?? null;
		this.sequence = params.sequence ?? null;
		this.timeoutSeconds = params.timeoutSeconds ?? 30;
		this.maxRetries = params.maxRetries ?? 5;
		this.retryStrategy = params.retryStrategy ?? null;
		this.scheduledFor = params.scheduledFor ?? null;
		this.expiresAt = params.expiresAt ?? null;
		this.idempotencyKey = params.idempotencyKey ?? null;
		this.externalId = params.externalId ?? null;
		this.connectionId = params.connectionId ?? null;
		this.queue = params.queue ?? null;
	}

	/**
	 * Create a new dispatch job DTO.
	 *
	 * @param code - Fully qualified `application:subdomain:aggregate:action`
	 *   code — the platform facets on its segments and resolves delivery
	 *   signing credentials from the application segment; bare codes are
	 *   rejected.
	 * @param payload - The payload string. If you have an object, JSON.stringify it first.
	 */
	static create(
		source: string,
		code: string,
		targetUrl: string,
		payload: string | Record<string, unknown>,
		dispatchPoolId: string,
	): CreateDispatchJobDto {
		assertQualifiedCode(code, "Dispatch job code");
		return new CreateDispatchJobDto({
			source,
			code,
			targetUrl,
			payload: typeof payload === "string" ? payload : JSON.stringify(payload),
			dispatchPoolId,
		});
	}

	withSubject(subject: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), subject });
	}

	withCorrelationId(correlationId: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), correlationId });
	}

	withEventId(eventId: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), eventId });
	}

	/** The metadata as an object (a view: JavaScript orders integer-like keys first). */
	get metadata(): Record<string, string> {
		return Object.fromEntries(this.metadataPairs);
	}

	/**
	 * Add metadata. An existing key keeps its place and takes the new value; a
	 * new key goes last. Pairs are sent in this order. Pass `(key, value)` to
	 * control order exactly: an object's own key order puts integer-like keys
	 * first.
	 */
	withMetadata(key: string, value: string): CreateDispatchJobDto;
	withMetadata(metadata: Record<string, string>): CreateDispatchJobDto;
	withMetadata(
		keyOrMetadata: string | Record<string, string>,
		value?: string,
	): CreateDispatchJobDto {
		const added: Array<[string, string]> =
			typeof keyOrMetadata === "string"
				? [[keyOrMetadata, value ?? ""]]
				: Object.entries(keyOrMetadata);
		const pairs = this.metadataPairs.map(([k, v]) => [k, v] as [string, string]);
		for (const [k, v] of added) {
			const at = pairs.findIndex(([existing]) => existing === k);
			if (at >= 0) pairs[at] = [k, v];
			else pairs.push([k, v]);
		}
		return new CreateDispatchJobDto({ ...this.toParams(), metadataPairs: pairs });
	}

	/** @deprecated Not sent: the platform does not take per-job headers. Kept so existing callers compile. */
	withHeaders(headers: Record<string, string>): CreateDispatchJobDto {
		return new CreateDispatchJobDto({
			...this.toParams(),
			headers: { ...this.headers, ...headers },
		});
	}

	withPayloadContentType(payloadContentType: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), payloadContentType });
	}

	withDataOnly(dataOnly: boolean): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), dataOnly });
	}

	withMessageGroup(messageGroup: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), messageGroup });
	}

	/** Ordering behavior within the message group; unset defaults to NEXT_ON_ERROR on the platform (in-sequence, moving on past a failure). */
	withMode(mode: DispatchMode): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), mode });
	}

	withSequence(sequence: number): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), sequence });
	}

	withTimeoutSeconds(timeoutSeconds: number): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), timeoutSeconds });
	}

	withMaxRetries(maxRetries: number): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), maxRetries });
	}

	/** @deprecated Not sent: the batch route does not take one. Kept so existing callers compile. */
	withRetryStrategy(retryStrategy: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), retryStrategy });
	}

	/**
	 * @deprecated Not sent: the batch route has no scheduledFor, so the job is
	 * created for immediate dispatch. Kept so existing callers compile.
	 */
	withScheduledFor(scheduledFor: Date): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), scheduledFor });
	}

	/** @deprecated Not sent: the batch route does not take one. Kept so existing callers compile. */
	withExpiresAt(expiresAt: Date): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), expiresAt });
	}

	/**
	 * Makes the create idempotent: the first job with a key, within its
	 * application (the code's first segment) and client, wins it; a repeat
	 * answers that job's id and creates nothing. At most 100 characters.
	 */
	withIdempotencyKey(idempotencyKey: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), idempotencyKey });
	}

	withExternalId(externalId: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), externalId });
	}

	/**
	 * The connection the job is delivered through: pausing it holds the job,
	 * and (with no subscription) its service account signs the delivery.
	 * Must be global or the job's client's, shared or the job's application's.
	 */
	withConnectionId(connectionId: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), connectionId });
	}

	/**
	 * The job's own dispatch priority: `DEFAULT` or `HIGH_PRIORITY`, matched
	 * ignoring case. Unset stays absent — never silently defaulted — so "not
	 * asked for" stays distinguishable from an explicit `DEFAULT`. Wins over
	 * the target subscription's own priority at publish time when set.
	 *
	 * Not validated here: the platform rejects an invalid value, and
	 * duplicating that check client-side would just be another place to drift.
	 */
	withQueue(queue: string): CreateDispatchJobDto {
		return new CreateDispatchJobDto({ ...this.toParams(), queue });
	}

	/** Build the dispatch job payload for the outbox. Filters out null values. */
	toPayload(): Record<string, unknown> {
		return filterNulls({
			source: this.source,
			code: this.code,
			targetUrl: this.targetUrl,
			payload: this.payload,
			payloadContentType: this.payloadContentType,
			dispatchPoolId: this.dispatchPoolId,
			subject: this.subject,
			correlationId: this.correlationId,
			eventId: this.eventId,
			// The platform takes a list of {key, value}; an object fails the whole batch.
			metadata:
				this.metadataPairs.length > 0
					? this.metadataPairs.map(([key, value]) => ({ key, value: String(value) }))
					: null,
			dataOnly: this.dataOnly,
			messageGroup: this.messageGroup,
			mode: this.mode,
			sequence: this.sequence,
			timeoutSeconds: this.timeoutSeconds,
			maxRetries: this.maxRetries,
			idempotencyKey: this.idempotencyKey,
			externalId: this.externalId,
			connectionId: this.connectionId,
			queue: this.queue,
		});
	}

	private toParams() {
		return {
			source: this.source,
			code: this.code,
			targetUrl: this.targetUrl,
			payload: this.payload,
			dispatchPoolId: this.dispatchPoolId,
			subject: this.subject,
			correlationId: this.correlationId,
			eventId: this.eventId,
			metadataPairs: this.metadataPairs,
			headers: this.headers,
			payloadContentType: this.payloadContentType,
			dataOnly: this.dataOnly,
			messageGroup: this.messageGroup,
			mode: this.mode,
			sequence: this.sequence,
			timeoutSeconds: this.timeoutSeconds,
			maxRetries: this.maxRetries,
			retryStrategy: this.retryStrategy,
			scheduledFor: this.scheduledFor,
			expiresAt: this.expiresAt,
			idempotencyKey: this.idempotencyKey,
			externalId: this.externalId,
			connectionId: this.connectionId,
			queue: this.queue,
		};
	}
}

function filterNulls(obj: Record<string, unknown>): Record<string, unknown> {
	const result: Record<string, unknown> = {};
	for (const [key, value] of Object.entries(obj)) {
		if (value !== null && value !== undefined) {
			result[key] = value;
		}
	}
	return result;
}
