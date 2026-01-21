package tech.flowcatalyst.platform.bff;

import io.quarkus.runtime.annotations.RegisterForReflection;
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
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformMessagingPermissions;

import java.util.List;

/**
 * Debug BFF resource for querying raw dispatch jobs from the transactional collection.
 *
 * This endpoint is for admin/debug purposes only. The raw dispatch_jobs collection has
 * minimal indexes optimized for writes. For regular UI queries, use /api/bff/dispatch-jobs
 * which queries the dispatch_jobs_read projection.
 */
@Path("/api/bff/debug/dispatch-jobs")
@Produces(MediaType.APPLICATION_JSON)
@Tag(name = "Raw Dispatch Jobs (Debug)", description = "Debug endpoint for raw dispatch job queries")
@RegisterForReflection(registerFullHierarchy = true)
public class RawDispatchJobBffResource {

    @Inject
    DispatchJobRepository dispatchJobRepository;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    @GET
    @Operation(summary = "List raw dispatch jobs", description = "List raw dispatch jobs from the transactional collection (debug/admin only)")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Dispatch jobs found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON,
                schema = @Schema(implementation = PagedRawDispatchJobResponse.class))
        )
    })
    public Response listRawDispatchJobs(
        @QueryParam("page") @DefaultValue("0") Integer page,
        @QueryParam("size") @DefaultValue("20") Integer size
    ) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW_RAW);

        // Validate pagination
        if (page < 0) page = 0;
        if (size < 1 || size > 100) size = 20;

        // Query raw dispatch jobs - limited filtering since this is debug
        List<DispatchJob> jobs = dispatchJobRepository.findRecentPaged(page, size);

        long totalCount = dispatchJobRepository.count();

        List<RawDispatchJobResponse> responses = jobs.stream()
            .map(RawDispatchJobResponse::from)
            .toList();

        return Response.ok(new PagedRawDispatchJobResponse(
            responses,
            page,
            size,
            totalCount,
            (int) Math.ceil((double) totalCount / size)
        )).build();
    }

    @GET
    @Path("/{id}")
    @Operation(summary = "Get raw dispatch job by ID", description = "Get a single raw dispatch job by its ID (debug/admin only)")
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Dispatch job found"),
        @APIResponse(responseCode = "404", description = "Dispatch job not found")
    })
    public Response getRawDispatchJob(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW_RAW);

        DispatchJob job = dispatchJobRepository.findById(id);
        if (job == null) {
            return Response.status(404)
                .entity(new ErrorResponse("Dispatch job not found: " + id))
                .build();
        }
        return Response.ok(RawDispatchJobResponse.from(job)).build();
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record RawDispatchJobResponse(
        String id,
        String externalId,
        String source,
        String kind,
        String code,
        String subject,
        String eventId,
        String correlationId,
        String targetUrl,
        String protocol,
        String clientId,
        String subscriptionId,
        String serviceAccountId,
        String dispatchPoolId,
        String messageGroup,
        String mode,
        int sequence,
        String status,
        int attemptCount,
        int maxRetries,
        String lastError,
        int timeoutSeconds,
        String retryStrategy,
        String idempotencyKey,
        String createdAt,
        String updatedAt,
        String scheduledFor,
        String completedAt,
        // Include payload info for debug (but not full payload)
        String payloadContentType,
        int payloadLength,
        int attemptHistoryCount
    ) {
        public static RawDispatchJobResponse from(DispatchJob job) {
            return new RawDispatchJobResponse(
                job.id,
                job.externalId,
                job.source,
                job.kind != null ? job.kind.name() : null,
                job.code,
                job.subject,
                job.eventId,
                job.correlationId,
                job.targetUrl,
                job.protocol != null ? job.protocol.name() : null,
                job.clientId,
                job.subscriptionId,
                job.serviceAccountId,
                job.dispatchPoolId,
                job.messageGroup,
                job.mode != null ? job.mode.name() : null,
                job.sequence,
                job.status != null ? job.status.name() : null,
                job.attemptCount != null ? job.attemptCount : 0,
                job.maxRetries != null ? job.maxRetries : 3,
                job.lastError,
                job.timeoutSeconds,
                job.retryStrategy,
                job.idempotencyKey,
                job.createdAt != null ? job.createdAt.toString() : null,
                job.updatedAt != null ? job.updatedAt.toString() : null,
                job.scheduledFor != null ? job.scheduledFor.toString() : null,
                job.completedAt != null ? job.completedAt.toString() : null,
                job.payloadContentType,
                job.payload != null ? job.payload.length() : 0,
                job.attempts != null ? job.attempts.size() : 0
            );
        }
    }

    public record PagedRawDispatchJobResponse(
        List<RawDispatchJobResponse> items,
        int page,
        int size,
        long totalItems,
        int totalPages
    ) {}

    public record ErrorResponse(String error) {}
}
