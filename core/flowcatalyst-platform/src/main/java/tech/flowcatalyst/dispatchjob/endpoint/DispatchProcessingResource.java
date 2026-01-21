package tech.flowcatalyst.dispatchjob.endpoint;

import com.fasterxml.jackson.annotation.JsonProperty;
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
import tech.flowcatalyst.dispatchjob.security.DispatchAuthService;
import tech.flowcatalyst.dispatchjob.service.DispatchJobService;

@Path("/api/dispatch/process")
@Tag(name = "Dispatch Processing", description = "Internal endpoint for processing dispatch jobs via message router")
public class DispatchProcessingResource {

    private static final Logger LOG = Logger.getLogger(DispatchProcessingResource.class);

    @Inject
    DispatchJobService dispatchJobService;

    @Inject
    DispatchAuthService dispatchAuthService;

    @POST
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Process a dispatch job (internal endpoint called by message router)",
        description = "Internal endpoint that executes webhook dispatch and records attempts. " +
            "Requires HMAC-SHA256 authentication via Bearer token.")
    @APIResponses({
        @APIResponse(
            responseCode = "200",
            description = "Job processed (check ack field for success/failure)",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ProcessResponse.class))
        ),
        @APIResponse(
            responseCode = "401",
            description = "Invalid or missing authentication token",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ProcessResponse.class))
        ),
        @APIResponse(
            responseCode = "500",
            description = "Internal error during processing",
            content = @Content(mediaType = MediaType.APPLICATION_JSON, schema = @Schema(implementation = ProcessResponse.class))
        )
    })
    public Response processDispatchJob(
            ProcessRequest request,
            @HeaderParam("Authorization") String authHeader) {

        LOG.infof("Received dispatch job processing request: %s", request.messageId());

        // 1. Extract and validate auth token
        String token = extractBearerToken(authHeader);
        if (token == null) {
            LOG.warnf("Dispatch process request missing Authorization header for message [%s]", request.messageId());
            return Response.status(401)
                .entity(ProcessResponse.nack("Missing Authorization header"))
                .build();
        }

        if (!dispatchAuthService.validateAuthToken(request.messageId(), token)) {
            LOG.warnf("Dispatch process auth failed for message [%s]", request.messageId());
            return Response.status(401)
                .entity(ProcessResponse.nack("Invalid auth token"))
                .build();
        }

        // 2. Process the dispatch job
        try {
            String dispatchJobId = request.messageId();

            DispatchJobService.DispatchJobProcessResult result = dispatchJobService.processDispatchJob(dispatchJobId);

            return Response.status(200)
                .entity(new ProcessResponse(result.ack(), result.message(), result.delaySeconds()))
                .build();

        } catch (IllegalArgumentException e) {
            // Job not found - ACK it since we can't process (similar to Laravel behavior)
            LOG.warnf("Dispatch job not found: %s", request.messageId());
            return Response.status(200)
                .entity(ProcessResponse.ack("Cannot find record."))
                .build();

        } catch (Exception e) {
            // Infrastructure error - return 500, message router will retry via visibility timeout
            LOG.errorf(e, "Error processing dispatch job: %s", request.messageId());
            return Response.status(500)
                .entity(new ProcessResponse(false, e.getMessage(), null))
                .build();
        }
    }

    /**
     * Extract Bearer token from Authorization header.
     */
    private String extractBearerToken(String authHeader) {
        if (authHeader != null && authHeader.startsWith("Bearer ")) {
            return authHeader.substring(7).trim();
        }
        return null;
    }

    /**
     * Request from message router to process a dispatch job.
     */
    public record ProcessRequest(
        @JsonProperty("messageId") String messageId
    ) {
    }

    /**
     * Response to message router indicating processing result.
     *
     * <p>Aligns with MediationResponse contract:
     * <ul>
     *   <li><b>ack: true</b> - Remove from queue (success OR permanent error like max retries reached)</li>
     *   <li><b>ack: false</b> - Keep on queue, retry later (transient errors, not ready yet)</li>
     *   <li><b>delaySeconds</b> - Optional delay before message becomes visible again (for transient errors with backoff)</li>
     * </ul>
     */
    public record ProcessResponse(
        @JsonProperty("ack") boolean ack,
        @JsonProperty("message") String message,
        @JsonProperty("delaySeconds") Integer delaySeconds
    ) {
        /**
         * Create a response that acknowledges (removes from queue).
         */
        public static ProcessResponse ack(String message) {
            return new ProcessResponse(true, message, null);
        }

        /**
         * Create a response that does not acknowledge (keeps on queue for retry).
         */
        public static ProcessResponse nack(String message) {
            return new ProcessResponse(false, message, null);
        }

        /**
         * Create a response that does not acknowledge with a specific retry delay.
         */
        public static ProcessResponse nackWithDelay(String message, int delaySeconds) {
            return new ProcessResponse(false, message, delaySeconds);
        }
    }
}
