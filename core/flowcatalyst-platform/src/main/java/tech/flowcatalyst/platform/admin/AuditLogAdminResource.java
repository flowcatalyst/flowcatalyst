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

import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformAdminPermissions;
import tech.flowcatalyst.platform.common.api.ApiResponses;
import tech.flowcatalyst.platform.common.api.AuditLogResponses.*;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;

import java.util.List;

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
@jakarta.transaction.Transactional
public class AuditLogAdminResource {

    @Inject
    AuditLogRepository auditLogRepo;

    @Inject
    AuditContext auditContext;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    AuthorizationService authorizationService;

    // ==================== List Operations ====================

    /**
     * List audit logs with optional filtering and pagination.
     */
    @GET
    @Operation(operationId = "listAuditLogs", summary = "List audit logs",
        description = "Returns audit logs with optional filtering by entity type, entity ID, principal, or operation")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Audit logs retrieved",
            content = @Content(schema = @Schema(implementation = AuditLogListResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response listAuditLogs(
            @Parameter(description = "Filter by entity type (e.g., 'IdentityProvider', 'Role')")
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

        String authPrincipalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(authPrincipalId, PlatformAdminPermissions.AUDIT_LOG_VIEW);

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

        var dtos = logs.stream().map(this::toDto).toList();

        return Response.ok(new AuditLogListResponse(dtos, total, page, pageSize)).build();
    }

    /**
     * Get a specific audit log entry by ID.
     */
    @GET
    @Path("/{id}")
    @Operation(operationId = "getAuditLog", summary = "Get audit log by ID")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Audit log retrieved",
            content = @Content(schema = @Schema(implementation = AuditLogDetailDto.class))),
        @APIResponse(responseCode = "404", description = "Audit log not found",
            content = @Content(schema = @Schema(implementation = ApiResponses.NotFoundResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getAuditLog(@PathParam("id") String id) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.AUDIT_LOG_VIEW);

        AuditLog log = auditLogRepo.findById(id);
        if (log == null) {
            return Response.status(Response.Status.NOT_FOUND)
                .entity(new ApiResponses.NotFoundResponse("auditLog", id))
                .build();
        }

        return Response.ok(toDetailDto(log)).build();
    }

    /**
     * Get audit logs for a specific entity.
     */
    @GET
    @Path("/entity/{entityType}/{entityId}")
    @Operation(operationId = "getEntityAuditLogs", summary = "Get audit logs for entity",
        description = "Returns all audit logs for a specific entity")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Audit logs retrieved",
            content = @Content(schema = @Schema(implementation = EntityAuditLogsResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getAuditLogsForEntity(
            @PathParam("entityType") String entityType,
            @PathParam("entityId") String entityId) {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.AUDIT_LOG_VIEW);

        List<AuditLog> logs = auditLogRepo.findByEntity(entityType, entityId);
        var dtos = logs.stream().map(this::toDto).toList();

        return Response.ok(new EntityAuditLogsResponse(dtos, logs.size(), entityType, entityId)).build();
    }

    /**
     * Get distinct entity types that have audit logs.
     */
    @GET
    @Path("/entity-types")
    @Operation(operationId = "getAuditLogEntityTypes", summary = "Get entity types with audit logs",
        description = "Returns distinct entity types that have audit log entries")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Entity types retrieved",
            content = @Content(schema = @Schema(implementation = EntityTypesResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getEntityTypes() {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.AUDIT_LOG_VIEW);

        // Get distinct entity types using aggregation
        List<String> entityTypes = auditLogRepo.findDistinctEntityTypes();

        return Response.ok(new EntityTypesResponse(entityTypes)).build();
    }

    /**
     * Get distinct operations that have audit logs.
     */
    @GET
    @Path("/operations")
    @Operation(operationId = "getAuditLogOperations", summary = "Get operations with audit logs",
        description = "Returns distinct operation names that have audit log entries")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Operations retrieved",
            content = @Content(schema = @Schema(implementation = OperationsResponse.class))),
        @APIResponse(responseCode = "401", description = "Not authenticated",
            content = @Content(schema = @Schema(implementation = ApiResponses.UnauthorizedResponse.class))),
        @APIResponse(responseCode = "403", description = "Insufficient permissions",
            content = @Content(schema = @Schema(implementation = ApiResponses.ForbiddenResponse.class)))
    })
    public Response getOperations() {

        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformAdminPermissions.AUDIT_LOG_VIEW);

        // Get distinct operations using aggregation
        List<String> operations = auditLogRepo.findDistinctOperations();

        return Response.ok(new OperationsResponse(operations)).build();
    }

    // ==================== Helper Methods ====================

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
        // Prefer name, fall back to email from userIdentity
        if (principal.name != null && !principal.name.isBlank()) {
            return principal.name;
        }
        if (principal.userIdentity != null && principal.userIdentity.email != null) {
            return principal.userIdentity.email;
        }
        return "Unknown";
    }
}
