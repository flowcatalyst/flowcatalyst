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
export class CreateEventDto {
    constructor(params) {
        this.type = params.type;
        this.data = params.data;
        this.source = params.source ?? null;
        this.subject = params.subject ?? null;
        this.correlationId = params.correlationId ?? null;
        this.causationId = params.causationId ?? null;
        this.deduplicationId = params.deduplicationId ?? null;
        this.messageGroup = params.messageGroup ?? null;
        this.contextData = params.contextData ?? [];
        this.headers = params.headers ?? {};
    }
    static create(type, data) {
        return new CreateEventDto({ type, data });
    }
    withSource(source) {
        return new CreateEventDto({ ...this.toParams(), source });
    }
    withSubject(subject) {
        return new CreateEventDto({ ...this.toParams(), subject });
    }
    withCorrelationId(correlationId) {
        return new CreateEventDto({ ...this.toParams(), correlationId });
    }
    withCausationId(causationId) {
        return new CreateEventDto({ ...this.toParams(), causationId });
    }
    withDeduplicationId(deduplicationId) {
        return new CreateEventDto({ ...this.toParams(), deduplicationId });
    }
    withMessageGroup(messageGroup) {
        return new CreateEventDto({ ...this.toParams(), messageGroup });
    }
    withHeaders(headers) {
        return new CreateEventDto({
            ...this.toParams(),
            headers: { ...this.headers, ...headers },
        });
    }
    withContextData(contextData) {
        return new CreateEventDto({
            ...this.toParams(),
            contextData: [...this.contextData, ...contextData],
        });
    }
    /** Build the event payload for the outbox. Filters out null values. */
    toPayload() {
        return filterNulls({
            specVersion: "1.0",
            type: this.type,
            source: this.source,
            subject: this.subject,
            data: JSON.stringify(this.data),
            correlationId: this.correlationId,
            causationId: this.causationId,
            deduplicationId: this.deduplicationId,
            messageGroup: this.messageGroup,
            contextData: this.contextData.length > 0 ? this.contextData : null,
        });
    }
    toParams() {
        return {
            type: this.type,
            data: this.data,
            source: this.source,
            subject: this.subject,
            correlationId: this.correlationId,
            causationId: this.causationId,
            deduplicationId: this.deduplicationId,
            messageGroup: this.messageGroup,
            contextData: this.contextData,
            headers: this.headers,
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
