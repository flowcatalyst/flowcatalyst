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
export declare class CreateAuditLogDto {
    readonly entityType: string;
    readonly entityId: string;
    readonly operation: string;
    readonly operationData: Record<string, unknown> | null;
    readonly principalId: string | null;
    readonly performedAt: Date | null;
    readonly source: string | null;
    readonly correlationId: string | null;
    readonly metadata: Record<string, string>;
    readonly headers: Record<string, string>;
    private constructor();
    static create(entityType: string, entityId: string, operation: string): CreateAuditLogDto;
    withOperationData(operationData: Record<string, unknown>): CreateAuditLogDto;
    withPrincipalId(principalId: string): CreateAuditLogDto;
    withPerformedAt(performedAt: Date): CreateAuditLogDto;
    withSource(source: string): CreateAuditLogDto;
    withCorrelationId(correlationId: string): CreateAuditLogDto;
    withMetadata(metadata: Record<string, string>): CreateAuditLogDto;
    withHeaders(headers: Record<string, string>): CreateAuditLogDto;
    /** Build the audit log payload for the outbox. Filters out null values. */
    toPayload(): Record<string, unknown>;
    private toParams;
}
//# sourceMappingURL=create-audit-log-dto.d.ts.map