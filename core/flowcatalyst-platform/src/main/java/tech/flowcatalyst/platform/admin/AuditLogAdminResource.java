package tech.flowcatalyst.platform.admin;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.parameters.Parameter;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.audit.AuditLog;
import tech.flowcatalyst.platform.audit.AuditLogRepository;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * Admin API for viewing audit logs.
 *
 * Provides read-only access to the audit trail of operations performed in the system.
 * Audit logs track entity changes with full operation payloads for compliance and debugging.
 */
@Path("/api/admin/audit-logs")
@Tag(name = "BFF - Audit Log Admin", description = "Audit log viewing endpoints")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class AuditLogAdminResource {

    @Inject
    AuditLogRepository auditLogRepo;

    @Inject
    AuditContext auditContext;

    @Inject
    PrincipalRepository principalRepo;

    // ==================== List Operations ====================

    /**
     * List audit logs with optional filtering and pagination.
     */
    @GET
    @Operation(summary = "List audit logs",
        description = "Returns audit logs with optional filtering by entity type, entity ID, principal, or operation")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Audit logs retrieved",
            content = @Content(schema = @Schema(implementation = AuditLogListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response listAuditLogs(
            @Parameter(description = "Filter by entity type (e.g., 'ClientAuthConfig', 'Role')")
            @QueryParam("entityType") String entityType,
            @Parameter(description = "Filter by entity ID")
            @QueryParam("entityId") String entityId,
            @Parameter(description = "Filter by principal ID")
            @QueryParam("principalId") String principalId,
            @Parameter(description = "Filter by operation name")
            @QueryParam("operation") String operation,
            @Parameter(description = "Page number (0-based)")
            @QueryParam("page") @DefaultValue("0") int page,
            @Parameter(description = "Page size")
            @QueryParam("pageSize") @DefaultValue("50") int pageSize) {

        auditContext.requirePrincipalId();

        List<AuditLog> logs;
        long total;

        // Apply filters
        if (entityType != null && entityId != null) {
            logs = auditLogRepo.findByEntity(entityType, entityId);
            total = logs.size();
        } else if (entityType != null) {
            logs = auditLogRepo.findByEntityTypePaged(entityType, page, pageSize);
            total = auditLogRepo.countByEntityType(entityType);
        } else if (principalId != null) {
            logs = auditLogRepo.findByPrincipal(principalId);
            total = logs.size();
        } else if (operation != null) {
            logs = auditLogRepo.findByOperation(operation);
            total = logs.size();
        } else {
            logs = auditLogRepo.findPaged(page, pageSize);
            total = auditLogRepo.count();
        }

        var response = logs.stream().map(this::toDto).toList();

        return Response.ok(Map.of(
            "auditLogs", response,
            "total", total,
            "page", page,
            "pageSize", pageSize
        )).build();
    }

    /**
     * Get a specific audit log entry by ID.
     */
    @GET
    @Path("/{id}")
    @Operation(summary = "Get audit log by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Audit log retrieved",
            content = @Content(schema = @Schema(implementation = AuditLogDto.class))),
        @APIResponse(responseCode = "404", description = "Audit log not found"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response getAuditLog(@PathParam("id") String id) {

        auditContext.requirePrincipalId();

        AuditLog log = auditLogRepo.findById(id);
        if (log == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ErrorResponse("Audit log not found"))
                .build();
        }

        return Response.ok(toDetailDto(log)).build();
    }

    /**
     * Get audit logs for a specific entity.
     */
    @GET
    @Path("/entity/{entityType}/{entityId}")
    @Operation(summary = "Get audit logs for entity",
        description = "Returns all audit logs for a specific entity")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Audit logs retrieved"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response getAuditLogsForEntity(
            @PathParam("entityType") String entityType,
            @PathParam("entityId") String entityId) {

        auditContext.requirePrincipalId();

        List<AuditLog> logs = auditLogRepo.findByEntity(entityType, entityId);
        var response = logs.stream().map(this::toDto).toList();

        return Response.ok(Map.of(
            "auditLogs", response,
            "total", logs.size(),
            "entityType", entityType,
            "entityId", entityId
        )).build();
    }

    /**
     * Get distinct entity types that have audit logs.
     */
    @GET
    @Path("/entity-types")
    @Operation(summary = "Get entity types with audit logs",
        description = "Returns distinct entity types that have audit log entries")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Entity types retrieved"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response getEntityTypes() {

        auditContext.requirePrincipalId();

        // Get distinct entity types using aggregation
        List<String> entityTypes = auditLogRepo.findDistinctEntityTypes();

        return Response.ok(Map.of("entityTypes", entityTypes)).build();
    }

    /**
     * Get distinct operations that have audit logs.
     */
    @GET
    @Path("/operations")
    @Operation(summary = "Get operations with audit logs",
        description = "Returns distinct operation names that have audit log entries")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Operations retrieved"),
        @APIResponse(responseCode = "401", description = "Not authenticated")
    })
    public Response getOperations() {

        auditContext.requirePrincipalId();

        // Get distinct operations using aggregation
        List<String> operations = auditLogRepo.findDistinctOperations();

        return Response.ok(Map.of("operations", operations)).build();
    }

    // ==================== DTOs ====================

    private AuditLogDto toDto(AuditLog log) {
        String principalName = resolvePrincipalName(log.principalId);

        return new AuditLogDto(
            log.id,
            log.entityType,
            log.entityId,
            log.operation,
            log.principalId,
            principalName,
            log.performedAt
        );
    }

    private AuditLogDetailDto toDetailDto(AuditLog log) {
        String principalName = resolvePrincipalName(log.principalId);

        return new AuditLogDetailDto(
            log.id,
            log.entityType,
            log.entityId,
            log.operation,
            log.operationJson,
            log.principalId,
            principalName,
            log.performedAt
        );
    }

    private String resolvePrincipalName(String principalId) {
        if (principalId == null) {
            return null;
        }
        Principal principal = principalRepo.findById(principalId);
        if (principal == null) {
            return null;
        }
        // Prefer name, fall back to email from userIdentity, fall back to service account code
        if (principal.name != null && !principal.name.isBlank()) {
            return principal.name;
        }
        if (principal.userIdentity != null && principal.userIdentity.email != null) {
            return principal.userIdentity.email;
        }
        if (principal.serviceAccount != null && principal.serviceAccount.code != null) {
            return principal.serviceAccount.code;
        }
        return "Unknown";
    }

    // ==================== Record Types ====================

    public record AuditLogDto(
        String id,
        String entityType,
        String entityId,
        String operation,
        String principalId,
        String principalName,
        Instant performedAt
    ) {}

    public record AuditLogDetailDto(
        String id,
        String entityType,
        String entityId,
        String operation,
        String operationJson,
        String principalId,
        String principalName,
        Instant performedAt
    ) {}

    public record AuditLogListResponse(
        List<AuditLogDto> auditLogs,
        long total,
        int page,
        int pageSize
    ) {}

    public record ErrorResponse(String error) {}
}
