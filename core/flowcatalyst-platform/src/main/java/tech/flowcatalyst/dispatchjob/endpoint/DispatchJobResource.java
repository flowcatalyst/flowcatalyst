package tech.flowcatalyst.dispatchjob.endpoint;

import jakarta.inject.Inject;
import jakarta.validation.Valid;
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
import tech.flowcatalyst.dispatchjob.dto.*;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.model.DispatchKind;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;
import tech.flowcatalyst.dispatchjob.service.DispatchJobService;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.platform.PlatformMessagingPermissions;

import java.time.Instant;
import java.util.List;

@Path("/api/dispatch/jobs")
@Tag(name = "Dispatch Jobs", description = "Endpoints for managing dispatch jobs")
public class DispatchJobResource {

    private static final Logger LOG = Logger.getLogger(DispatchJobResource.class);

    @Inject
    DispatchJobService dispatchJobService;

    @Inject
    AuditContext auditContext;

    @Inject
    AuthorizationService authorizationService;

    @POST
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Create a new dispatch job", description = "Creates and queues a new dispatch job for webhook delivery")
    @APIResponses({
        @APIResponse(
            responseCode = "201",
            description = "Dispatch job created successfully",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = DispatchJobResponse.class))
        ),
        @APIResponse(
            responseCode = "400",
            description = "Invalid request - missing or invalid fields",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ErrorResponse.class))
        ),
        @APIResponse(
            responseCode = "500",
            description = "Internal server error",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ErrorResponse.class))
        )
    })
    public Response createDispatchJob(@Valid CreateDispatchJobRequest request) {
        // Authorization - require DISPATCH_JOB_CREATE permission
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_CREATE);

        LOG.infof("Creating dispatch job: kind=%s, code=%s, source=%s, principal=%s",
            request.kind(), request.code(), request.source(), principalId);

        try {
            DispatchJob job = dispatchJobService.createDispatchJob(request);
            // Note: DO NOT create audit log for dispatch job creation (high volume)
            return Response.status(201).entity(DispatchJobResponse.from(job)).build();

        } catch (IllegalArgumentException e) {
            LOG.errorf("Invalid request: %s", e.getMessage());
            return Response.status(400).entity(new ErrorResponse(e.getMessage())).build();

        } catch (Exception e) {
            LOG.errorf(e, "Error creating dispatch job");
            return Response.status(500).entity(new ErrorResponse("Internal error: " + e.getMessage())).build();
        }
    }

    @POST
    @Path("/batch")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Create multiple dispatch jobs in batch", description = "Creates multiple dispatch jobs in a single operation. Maximum batch size is 100 jobs.")
    @APIResponses({
        @APIResponse(
            responseCode = "201",
            description = "Dispatch jobs created successfully",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = BatchDispatchJobResponse.class))
        ),
        @APIResponse(
            responseCode = "400",
            description = "Invalid request or batch size exceeds limit",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ErrorResponse.class))
        ),
        @APIResponse(
            responseCode = "500",
            description = "Internal server error",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ErrorResponse.class))
        )
    })
    public Response createDispatchJobBatch(List<@Valid CreateDispatchJobRequest> requests) {
        // Authorization - require DISPATCH_JOB_CREATE permission
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_CREATE);

        if (requests == null || requests.isEmpty()) {
            return Response.status(400).entity(new ErrorResponse("Request body must contain at least one dispatch job")).build();
        }
        if (requests.size() > 100) {
            return Response.status(400).entity(new ErrorResponse("Batch size cannot exceed 100 dispatch jobs")).build();
        }

        LOG.infof("Creating batch of %d dispatch jobs, principal=%s", requests.size(), principalId);

        try {
            List<DispatchJobResponse> results = requests.stream()
                .map(request -> {
                    DispatchJob job = dispatchJobService.createDispatchJob(request);
                    return DispatchJobResponse.from(job);
                })
                .toList();

            return Response.status(201).entity(new BatchDispatchJobResponse(results, results.size())).build();

        } catch (IllegalArgumentException e) {
            LOG.errorf("Invalid batch request: %s", e.getMessage());
            return Response.status(400).entity(new ErrorResponse(e.getMessage())).build();

        } catch (Exception e) {
            LOG.errorf(e, "Error creating dispatch job batch");
            return Response.status(500).entity(new ErrorResponse("Internal error: " + e.getMessage())).build();
        }
    }

    @GET
    @Path("/{id}")
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Get dispatch job by ID", description = "Retrieves detailed information about a specific dispatch job")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Dispatch job found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = DispatchJobResponse.class))
        ),
        @APIResponse(
            responseCode = "404",
            description = "Dispatch job not found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ErrorResponse.class))
        )
    })
    public Response getDispatchJob(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW);

        return dispatchJobService.findById(id)
            .map(job -> Response.ok(DispatchJobResponse.from(job)).build())
            .orElse(Response.status(404).entity(new ErrorResponse("Dispatch job not found")).build());
    }

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Search and filter dispatch jobs", description = "Search for dispatch jobs with optional filters and pagination")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Search results returned",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = PagedDispatchJobResponse.class))
        )
    })
    public Response searchDispatchJobs(
        @QueryParam("status") DispatchStatus status,
        @QueryParam("source") String source,
        @QueryParam("kind") DispatchKind kind,
        @QueryParam("code") String code,
        @QueryParam("clientId") String clientId,
        @QueryParam("subscriptionId") String subscriptionId,
        @QueryParam("dispatchPoolId") String dispatchPoolId,
        @QueryParam("messageGroup") String messageGroup,
        @QueryParam("createdAfter") Instant createdAfter,
        @QueryParam("createdBefore") Instant createdBefore,
        @QueryParam("page") @DefaultValue("0") Integer page,
        @QueryParam("size") @DefaultValue("20") Integer size
    ) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW);

        DispatchJobFilter filter = new DispatchJobFilter(
            status, source, kind, code, clientId, subscriptionId, dispatchPoolId, messageGroup,
            createdAfter, createdBefore, page, size
        );

        List<DispatchJob> jobs = dispatchJobService.findWithFilter(filter);
        long totalCount = dispatchJobService.countWithFilter(filter);

        List<DispatchJobResponse> responses = jobs.stream()
            .map(DispatchJobResponse::from)
            .toList();

        return Response.ok(new PagedDispatchJobResponse(
            responses,
            page,
            size,
            totalCount,
            (int) Math.ceil((double) totalCount / size)
        )).build();
    }

    @GET
    @Path("/{id}/attempts")
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Get all attempts for a dispatch job", description = "Retrieves the full history of webhook delivery attempts for a job")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Attempts list returned",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = DispatchAttemptResponse.class))
        ),
        @APIResponse(
            responseCode = "404",
            description = "Dispatch job not found",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ErrorResponse.class))
        )
    })
    public Response getDispatchJobAttempts(@PathParam("id") String id) {
        String principalId = auditContext.requirePrincipalId();
        authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW);

        return dispatchJobService.findById(id)
            .map(job -> {
                List<DispatchAttemptResponse> responses = job.attempts.stream()
                    .map(DispatchAttemptResponse::from)
                    .toList();
                return Response.ok(responses).build();
            })
            .orElse(Response.status(404).entity(new ErrorResponse("Dispatch job not found")).build());
    }

    public record PagedDispatchJobResponse(
        List<DispatchJobResponse> items,
        int page,
        int size,
        long totalItems,
        int totalPages
    ) {
    }

    public record BatchDispatchJobResponse(
        List<DispatchJobResponse> jobs,
        int count
    ) {
    }
}
