/**
 * DTO for creating an event in the outbox.
 *
 * Uses an immutable builder pattern - all `with*()` methods return a new instance.
 *
 * @example
 * ```typescript
 * const event = CreateEventDto
 *   .create('user.registered', { userId: '123', email: 'a@b.com' })
 *   .withSource('user-service')
 *   .withCorrelationId('corr-456');
 * ```
 */
export declare class CreateEventDto {
    readonly type: string;
    readonly data: Record<string, unknown>;
    readonly source: string | null;
    readonly subject: string | null;
    readonly correlationId: string | null;
    readonly causationId: string | null;
    readonly deduplicationId: string | null;
    readonly messageGroup: string | null;
    readonly contextData: Array<{
        key: string;
        value: string;
    }>;
    readonly headers: Record<string, string>;
    private constructor();
    static create(type: string, data: Record<string, unknown>): CreateEventDto;
    withSource(source: string): CreateEventDto;
    withSubject(subject: string): CreateEventDto;
    withCorrelationId(correlationId: string): CreateEventDto;
    withCausationId(causationId: string): CreateEventDto;
    withDeduplicationId(deduplicationId: string): CreateEventDto;
    withMessageGroup(messageGroup: string): CreateEventDto;
    withHeaders(headers: Record<string, string>): CreateEventDto;
    withContextData(contextData: Array<{
        key: string;
        value: string;
    }>): CreateEventDto;
    /** Build the event payload for the outbox. Filters out null values. */
    toPayload(): Record<string, unknown>;
    private toParams;
}
//# sourceMappingURL=create-event-dto.d.ts.map