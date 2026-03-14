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
export declare class CreateDispatchJobDto {
    readonly source: string;
    readonly code: string;
    readonly targetUrl: string;
    readonly payload: string;
    readonly dispatchPoolId: string;
    readonly subject: string | null;
    readonly correlationId: string | null;
    readonly eventId: string | null;
    readonly metadata: Record<string, string>;
    readonly headers: Record<string, string>;
    readonly payloadContentType: string;
    readonly dataOnly: boolean;
    readonly messageGroup: string | null;
    readonly sequence: number | null;
    readonly timeoutSeconds: number;
    readonly maxRetries: number;
    readonly retryStrategy: string | null;
    readonly scheduledFor: Date | null;
    readonly expiresAt: Date | null;
    readonly idempotencyKey: string | null;
    readonly externalId: string | null;
    readonly connectionId: string | null;
    private constructor();
    /**
     * Create a new dispatch job DTO.
     *
     * @param payload - The payload string. If you have an object, JSON.stringify it first.
     */
    static create(source: string, code: string, targetUrl: string, payload: string | Record<string, unknown>, dispatchPoolId: string): CreateDispatchJobDto;
    withSubject(subject: string): CreateDispatchJobDto;
    withCorrelationId(correlationId: string): CreateDispatchJobDto;
    withEventId(eventId: string): CreateDispatchJobDto;
    withMetadata(metadata: Record<string, string>): CreateDispatchJobDto;
    withHeaders(headers: Record<string, string>): CreateDispatchJobDto;
    withPayloadContentType(payloadContentType: string): CreateDispatchJobDto;
    withDataOnly(dataOnly: boolean): CreateDispatchJobDto;
    withMessageGroup(messageGroup: string): CreateDispatchJobDto;
    withSequence(sequence: number): CreateDispatchJobDto;
    withTimeoutSeconds(timeoutSeconds: number): CreateDispatchJobDto;
    withMaxRetries(maxRetries: number): CreateDispatchJobDto;
    withRetryStrategy(retryStrategy: string): CreateDispatchJobDto;
    withScheduledFor(scheduledFor: Date): CreateDispatchJobDto;
    withExpiresAt(expiresAt: Date): CreateDispatchJobDto;
    withIdempotencyKey(idempotencyKey: string): CreateDispatchJobDto;
    withExternalId(externalId: string): CreateDispatchJobDto;
    withConnectionId(connectionId: string): CreateDispatchJobDto;
    /** Build the dispatch job payload for the outbox. Filters out null values. */
    toPayload(): Record<string, unknown>;
    private toParams;
}
//# sourceMappingURL=create-dispatch-job-dto.d.ts.map