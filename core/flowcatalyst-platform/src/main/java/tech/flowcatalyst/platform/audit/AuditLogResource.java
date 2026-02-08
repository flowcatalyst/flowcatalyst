package tech.flowcatalyst.platform.audit;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.List;

/**
 * REST resource for audit log ingestion from SDK outbox processor.
 */
@Path("/api/audit-logs")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@Tag(name = "Audit Logs", description = "Audit log ingestion from SDK outbox")
public class AuditLogResource {

    private static final Logger LOG = Logger.getLogger(AuditLogResource.class);

    @Inject
    AuditLogRepository auditLogRepo;

    @POST
    @Path("/batch")
    @Operation(summary = "Create multiple audit logs in batch",
        description = "Creates multiple audit log entries in a single operation. Maximum batch size is 100.")
    @APIResponses({
        @APIResponse(
            responseCode = "201",
            description = "Audit logs created successfully",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = BatchAuditLogResponse.class))
        ),
        @APIResponse(responseCode = "400", description = "Invalid request or batch size exceeds limit",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = ErrorResponse.class)))
    })
    public Response createBatch(List<CreateAuditLogRequest> requests) {
        if (requests == null || requests.isEmpty()) {
            return Response.status(400)
                .entity(new ErrorResponse("INVALID_REQUEST", "Request body must contain at least one audit log"))
                .build();
        }
        if (requests.size() > 100) {
            return Response.status(400)
                .entity(new ErrorResponse("BATCH_SIZE_EXCEEDED", "Batch size cannot exceed 100 audit logs"))
                .build();
        }

        int count = 0;
        for (var request : requests) {
            var auditLog = new AuditLog();
            auditLog.id = TsidGenerator.generate(EntityType.AUDIT_LOG);
            auditLog.entityType = request.entityType();
            auditLog.entityId = request.entityId();
            auditLog.operation = request.operation();
            auditLog.operationJson = request.operationData();
            auditLog.principalId = request.principalId();
            auditLog.performedAt = request.performedAt() != null ? request.performedAt() : Instant.now();

            auditLogRepo.persist(auditLog);
            count++;
        }

        LOG.debugf("Created %d audit log entries via batch", count);
        return Response.status(201).entity(new BatchAuditLogResponse(count)).build();
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record CreateAuditLogRequest(
        String entityType,
        String entityId,
        String operation,
        String operationData,
        String principalId,
        Instant performedAt,
        String source,
        String correlationId
    ) {}

    public record BatchAuditLogResponse(int count) {}

    public record ErrorResponse(String code, String message) {}
}
