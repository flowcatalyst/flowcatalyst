/**
 * DTO for creating an audit log entry in the outbox.
 *
 * Uses an immutable builder pattern - all `with*()` methods return a new instance.
 *
 * @example
 * ```typescript
 * const auditLog = CreateAuditLogDto
 *   .create('User', '0HZXEQ5Y8JY5Z', 'CREATE')
 *   .withOperationData({ email: 'user@example.com', name: 'John' })
 *   .withPrincipalId('0HZXEQ5Y8JY5A')
 *   .withSource('user-service');
 * ```
 */
export class CreateAuditLogDto {
    constructor(params) {
        this.entityType = params.entityType;
        this.entityId = params.entityId;
        this.operation = params.operation;
        this.operationData = params.operationData ?? null;
        this.principalId = params.principalId ?? null;
        this.performedAt = params.performedAt ?? null;
        this.source = params.source ?? null;
        this.correlationId = params.correlationId ?? null;
        this.metadata = params.metadata ?? {};
        this.headers = params.headers ?? {};
    }
    static create(entityType, entityId, operation) {
        return new CreateAuditLogDto({ entityType, entityId, operation });
    }
    withOperationData(operationData) {
        return new CreateAuditLogDto({ ...this.toParams(), operationData });
    }
    withPrincipalId(principalId) {
        return new CreateAuditLogDto({ ...this.toParams(), principalId });
    }
    withPerformedAt(performedAt) {
        return new CreateAuditLogDto({ ...this.toParams(), performedAt });
    }
    withSource(source) {
        return new CreateAuditLogDto({ ...this.toParams(), source });
    }
    withCorrelationId(correlationId) {
        return new CreateAuditLogDto({ ...this.toParams(), correlationId });
    }
    withMetadata(metadata) {
        return new CreateAuditLogDto({
            ...this.toParams(),
            metadata: { ...this.metadata, ...metadata },
        });
    }
    withHeaders(headers) {
        return new CreateAuditLogDto({
            ...this.toParams(),
            headers: { ...this.headers, ...headers },
        });
    }
    /** Build the audit log payload for the outbox. Filters out null values. */
    toPayload() {
        return filterNulls({
            entityType: this.entityType,
            entityId: this.entityId,
            operation: this.operation,
            operationData: this.operationData
                ? JSON.stringify(this.operationData)
                : null,
            principalId: this.principalId,
            performedAt: (this.performedAt ?? new Date()).toISOString(),
            source: this.source,
            correlationId: this.correlationId,
            metadata: Object.keys(this.metadata).length > 0 ? this.metadata : null,
        });
    }
    toParams() {
        return {
            entityType: this.entityType,
            entityId: this.entityId,
            operation: this.operation,
            operationData: this.operationData,
            principalId: this.principalId,
            performedAt: this.performedAt,
            source: this.source,
            correlationId: this.correlationId,
            metadata: this.metadata,
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
