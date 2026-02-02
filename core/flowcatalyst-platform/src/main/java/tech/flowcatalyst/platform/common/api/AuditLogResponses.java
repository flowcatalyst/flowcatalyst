package tech.flowcatalyst.platform.common.api;

import org.eclipse.microprofile.openapi.annotations.media.Schema;

import java.time.Instant;
import java.util.List;

/**
 * Response DTOs for Audit Log Admin API endpoints.
 */
public final class AuditLogResponses {

    private AuditLogResponses() {}

    // ========================================================================
    // Audit Log DTOs
    // ========================================================================

    @Schema(description = "Audit log summary")
    public record AuditLogDto(
        @Schema(description = "Audit log ID")
        String id,
        @Schema(description = "Entity type (e.g., 'Client', 'Application')")
        String entityType,
        @Schema(description = "Entity ID")
        String entityId,
        @Schema(description = "Operation performed")
        String operation,
        @Schema(description = "Principal ID who performed the operation")
        String principalId,
        @Schema(description = "Principal name (resolved)")
        String principalName,
        @Schema(description = "When the operation was performed")
        Instant performedAt
    ) {}

    @Schema(description = "Audit log with full details including operation payload")
    public record AuditLogDetailDto(
        @Schema(description = "Audit log ID")
        String id,
        @Schema(description = "Entity type")
        String entityType,
        @Schema(description = "Entity ID")
        String entityId,
        @Schema(description = "Operation performed")
        String operation,
        @Schema(description = "Full operation payload as JSON")
        String operationJson,
        @Schema(description = "Principal ID who performed the operation")
        String principalId,
        @Schema(description = "Principal name (resolved)")
        String principalName,
        @Schema(description = "When the operation was performed")
        Instant performedAt
    ) {}

    @Schema(description = "Paginated audit log list response")
    public record AuditLogListResponse(
        @Schema(description = "Audit log entries")
        List<AuditLogDto> auditLogs,
        @Schema(description = "Total count of matching entries")
        long total,
        @Schema(description = "Current page (0-based)")
        int page,
        @Schema(description = "Page size")
        int pageSize
    ) {}

    @Schema(description = "Audit logs for a specific entity")
    public record EntityAuditLogsResponse(
        @Schema(description = "Audit log entries")
        List<AuditLogDto> auditLogs,
        @Schema(description = "Total count")
        int total,
        @Schema(description = "Entity type")
        String entityType,
        @Schema(description = "Entity ID")
        String entityId
    ) {}

    @Schema(description = "List of distinct entity types")
    public record EntityTypesResponse(
        @Schema(description = "Distinct entity types with audit logs")
        List<String> entityTypes
    ) {}

    @Schema(description = "List of distinct operations")
    public record OperationsResponse(
        @Schema(description = "Distinct operations with audit logs")
        List<String> operations
    ) {}
}
