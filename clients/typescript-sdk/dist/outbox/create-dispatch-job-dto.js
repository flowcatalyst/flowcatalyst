/**
 * DTO for creating a dispatch job in the outbox.
 *
 * Uses an immutable builder pattern - all `with*()` methods return a new instance.
 *
 * @example
 * ```typescript
 * const job = CreateDispatchJobDto
 *   .create('order-service', 'order.process', 'https://api.example.com/webhook', '{"orderId":"123"}', 'pool-1')
 *   .withCorrelationId('corr-789')
 *   .withTimeoutSeconds(60);
 * ```
 */
export class CreateDispatchJobDto {
    constructor(params) {
        this.source = params.source;
        this.code = params.code;
        this.targetUrl = params.targetUrl;
        this.payload = params.payload;
        this.dispatchPoolId = params.dispatchPoolId;
        this.subject = params.subject ?? null;
        this.correlationId = params.correlationId ?? null;
        this.eventId = params.eventId ?? null;
        this.metadata = params.metadata ?? {};
        this.headers = params.headers ?? {};
        this.payloadContentType = params.payloadContentType ?? "application/json";
        this.dataOnly = params.dataOnly ?? true;
        this.messageGroup = params.messageGroup ?? null;
        this.sequence = params.sequence ?? null;
        this.timeoutSeconds = params.timeoutSeconds ?? 30;
        this.maxRetries = params.maxRetries ?? 5;
        this.retryStrategy = params.retryStrategy ?? null;
        this.scheduledFor = params.scheduledFor ?? null;
        this.expiresAt = params.expiresAt ?? null;
        this.idempotencyKey = params.idempotencyKey ?? null;
        this.externalId = params.externalId ?? null;
        this.connectionId = params.connectionId ?? null;
    }
    /**
     * Create a new dispatch job DTO.
     *
     * @param payload - The payload string. If you have an object, JSON.stringify it first.
     */
    static create(source, code, targetUrl, payload, dispatchPoolId) {
        return new CreateDispatchJobDto({
            source,
            code,
            targetUrl,
            payload: typeof payload === "string" ? payload : JSON.stringify(payload),
            dispatchPoolId,
        });
    }
    withSubject(subject) {
        return new CreateDispatchJobDto({ ...this.toParams(), subject });
    }
    withCorrelationId(correlationId) {
        return new CreateDispatchJobDto({ ...this.toParams(), correlationId });
    }
    withEventId(eventId) {
        return new CreateDispatchJobDto({ ...this.toParams(), eventId });
    }
    withMetadata(metadata) {
        return new CreateDispatchJobDto({
            ...this.toParams(),
            metadata: { ...this.metadata, ...metadata },
        });
    }
    withHeaders(headers) {
        return new CreateDispatchJobDto({
            ...this.toParams(),
            headers: { ...this.headers, ...headers },
        });
    }
    withPayloadContentType(payloadContentType) {
        return new CreateDispatchJobDto({ ...this.toParams(), payloadContentType });
    }
    withDataOnly(dataOnly) {
        return new CreateDispatchJobDto({ ...this.toParams(), dataOnly });
    }
    withMessageGroup(messageGroup) {
        return new CreateDispatchJobDto({ ...this.toParams(), messageGroup });
    }
    withSequence(sequence) {
        return new CreateDispatchJobDto({ ...this.toParams(), sequence });
    }
    withTimeoutSeconds(timeoutSeconds) {
        return new CreateDispatchJobDto({ ...this.toParams(), timeoutSeconds });
    }
    withMaxRetries(maxRetries) {
        return new CreateDispatchJobDto({ ...this.toParams(), maxRetries });
    }
    withRetryStrategy(retryStrategy) {
        return new CreateDispatchJobDto({ ...this.toParams(), retryStrategy });
    }
    withScheduledFor(scheduledFor) {
        return new CreateDispatchJobDto({ ...this.toParams(), scheduledFor });
    }
    withExpiresAt(expiresAt) {
        return new CreateDispatchJobDto({ ...this.toParams(), expiresAt });
    }
    withIdempotencyKey(idempotencyKey) {
        return new CreateDispatchJobDto({ ...this.toParams(), idempotencyKey });
    }
    withExternalId(externalId) {
        return new CreateDispatchJobDto({ ...this.toParams(), externalId });
    }
    withConnectionId(connectionId) {
        return new CreateDispatchJobDto({ ...this.toParams(), connectionId });
    }
    /** Build the dispatch job payload for the outbox. Filters out null values. */
    toPayload() {
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
            metadata: Object.keys(this.metadata).length > 0 ? this.metadata : null,
            headers: Object.keys(this.headers).length > 0 ? this.headers : null,
            dataOnly: this.dataOnly,
            messageGroup: this.messageGroup,
            sequence: this.sequence,
            timeoutSeconds: this.timeoutSeconds,
            maxRetries: this.maxRetries,
            retryStrategy: this.retryStrategy,
            scheduledFor: this.scheduledFor?.toISOString() ?? null,
            expiresAt: this.expiresAt?.toISOString() ?? null,
            idempotencyKey: this.idempotencyKey,
            externalId: this.externalId,
            connectionId: this.connectionId,
        });
    }
    toParams() {
        return {
            source: this.source,
            code: this.code,
            targetUrl: this.targetUrl,
            payload: this.payload,
            dispatchPoolId: this.dispatchPoolId,
            subject: this.subject,
            correlationId: this.correlationId,
            eventId: this.eventId,
            metadata: this.metadata,
            headers: this.headers,
            payloadContentType: this.payloadContentType,
            dataOnly: this.dataOnly,
            messageGroup: this.messageGroup,
            sequence: this.sequence,
            timeoutSeconds: this.timeoutSeconds,
            maxRetries: this.maxRetries,
            retryStrategy: this.retryStrategy,
            scheduledFor: this.scheduledFor,
            expiresAt: this.expiresAt,
            idempotencyKey: this.idempotencyKey,
            externalId: this.externalId,
            connectionId: this.connectionId,
        };
    }
}
function filterNulls(obj) {
    const result = {};
    for (const [key, value] of Object.entries(obj)) {
        if (value !== null && value !== undefined) {
            result[key] = value;
        }
    }
    return result;
}
