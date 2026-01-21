package tech.flowcatalyst.messagerouter.endpoint;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;

import java.util.Random;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

@Path("/api/test")
@Tag(
    name = "Test Endpoints",
    description = "Test endpoints that simulate downstream service responses and document the expected mediation target format. " +
                  "These endpoints return MediationResponse (ack: true|false) format that HttpMediator expects from real downstream services."
)
public class TestResponseResource {

    private static final Logger LOG = Logger.getLogger(TestResponseResource.class);
    private final Random random = new Random();
    private final AtomicInteger requestCounter = new AtomicInteger(0);
    private final ConcurrentHashMap<String, String> receivedMessages = new ConcurrentHashMap<>();
    private final ObjectMapper objectMapper = new ObjectMapper();

    @POST
    @Path("/fast")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Fast response endpoint (100ms)")
    public Response fast(String body) throws InterruptedException {
        int requestId = requestCounter.incrementAndGet();
        LOG.infof("Fast endpoint - Request #%d started", requestId);

        Thread.sleep(100);

        LOG.infof("Fast endpoint - Request #%d completed", requestId);
        return Response.ok()
            .entity("{\"status\":\"success\",\"endpoint\":\"fast\",\"requestId\":" + requestId + "}")
            .build();
    }

    @POST
    @Path("/slow")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Slow response endpoint (60 seconds)")
    public Response slow(String body) throws InterruptedException {
        int requestId = requestCounter.incrementAndGet();
        LOG.infof("Slow endpoint - Request #%d started", requestId);

        Thread.sleep(60000);

        LOG.infof("Slow endpoint - Request #%d completed", requestId);
        return Response.ok()
            .entity("{\"status\":\"success\",\"endpoint\":\"slow\",\"requestId\":" + requestId + "}")
            .build();
    }

    @POST
    @Path("/faulty")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Faulty endpoint (random success/400/500)")
    public Response faulty(String body) throws InterruptedException {
        int requestId = requestCounter.incrementAndGet();

        // Extract message ID from body
        String messageId = extractMessageId(body);
        LOG.infof("Faulty endpoint - Request #%d - Message ID: %s", requestId, messageId);

        // Check if this message was already received
        String previousValue = receivedMessages.putIfAbsent(messageId, body);
        if (previousValue != null) {
            LOG.errorf("DUPLICATE MESSAGE DETECTED! Message ID %s was already received. This should NOT happen!", messageId);
        }

        int randomValue = random.nextInt(100);
        Thread.sleep(100);

        // 60% success, 20% 400 error, 20% 500 error
        if (randomValue < 60) {
            LOG.infof("Faulty endpoint - Request #%d - Message ID: %s - returned 200", requestId, messageId);
            return Response.ok()
                .entity("{\"status\":\"success\",\"endpoint\":\"faulty\",\"requestId\":" + requestId + ",\"messageId\":\"" + messageId + "\"}")
                .build();
        } else if (randomValue < 80) {
            LOG.warnf("Faulty endpoint - Request #%d - Message ID: %s - returned 400", requestId, messageId);
            return Response.status(400)
                .entity("{\"status\":\"error\",\"endpoint\":\"faulty\",\"requestId\":" + requestId + ",\"messageId\":\"" + messageId + "\",\"error\":\"Bad Request\"}")
                .build();
        } else {
            LOG.errorf("Faulty endpoint - Request #%d - Message ID: %s - returned 500", requestId, messageId);
            return Response.status(500)
                .entity("{\"status\":\"error\",\"endpoint\":\"faulty\",\"requestId\":" + requestId + ",\"messageId\":\"" + messageId + "\",\"error\":\"Internal Server Error\"}")
                .build();
        }
    }

    private String extractMessageId(String body) {
        try {
            JsonNode node = objectMapper.readTree(body);
            if (node.has("id")) {
                return node.get("id").asText();
            }
            return "unknown";
        } catch (Exception e) {
            LOG.warnf("Could not extract message ID from body: %s", body);
            return "parse-error";
        }
    }

    @POST
    @Path("/fail")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Always fail endpoint (500)")
    public Response fail(String body) throws InterruptedException {
        int requestId = requestCounter.incrementAndGet();
        LOG.errorf("Fail endpoint - Request #%d returning 500", requestId);

        Thread.sleep(100);

        return Response.status(500)
            .entity("{\"status\":\"error\",\"endpoint\":\"fail\",\"requestId\":" + requestId + ",\"error\":\"Always fails\"}")
            .build();
    }

    @GET
    @Path("/stats")
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Get request statistics")
    public Response stats() {
        return Response.ok()
            .entity("{\"totalRequests\":" + requestCounter.get() + "}")
            .build();
    }

    @POST
    @Path("/stats/reset")
    @Operation(summary = "Reset request statistics")
    public Response resetStats() {
        int previous = requestCounter.getAndSet(0);
        receivedMessages.clear();
        return Response.ok()
            .entity("{\"previousCount\":" + previous + ",\"currentCount\":0}")
            .build();
    }

    @POST
    @Path("/success")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(
        summary = "Success endpoint - ready to process",
        description = "Returns 200 OK with MediationResponse indicating message is ready (ack: true)"
    )
    @APIResponse(
        responseCode = "200",
        description = "Message successfully processed and ready",
        content = @Content(
            mediaType = MediaType.APPLICATION_JSON,
            schema = @Schema(
                example = "{\"ack\":true,\"message\":\"\"}",
                description = "MediationResponse - ack: true means message is ready for processing"
            )
        )
    )
    public Response success(String body) {
        int requestId = requestCounter.incrementAndGet();
        LOG.infof("Success endpoint - Request #%d completed", requestId);
        return Response.ok()
            .entity("{\"ack\":true,\"message\":\"\"}")
            .build();
    }

    @POST
    @Path("/pending")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(
        summary = "Pending endpoint - not ready yet",
        description = "Returns 200 OK with MediationResponse indicating message is NOT ready (ack: false). " +
                      "This is useful for messages that should be retried later (e.g., notBefore time not reached)"
    )
    @APIResponse(
        responseCode = "200",
        description = "Message not ready yet - will be retried via queue visibility timeout",
        content = @Content(
            mediaType = MediaType.APPLICATION_JSON,
            schema = @Schema(
                example = "{\"ack\":false,\"message\":\"notBefore time not reached\"}",
                description = "MediationResponse - ack: false means message should be retried (transient state)"
            )
        )
    )
    public Response pending(String body) {
        int requestId = requestCounter.incrementAndGet();
        LOG.infof("Pending endpoint - Request #%d returning 200 with ack: false", requestId);
        return Response.ok()
            .entity("{\"ack\":false,\"message\":\"notBefore time not reached\"}")
            .build();
    }

    @POST
    @Path("/client-error")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Always client error endpoint (400)")
    public Response clientError(String body) {
        int requestId = requestCounter.incrementAndGet();
        LOG.warnf("Client error endpoint - Request #%d returning 400", requestId);
        return Response.status(400)
            .entity("{\"status\":\"error\",\"endpoint\":\"client-error\",\"requestId\":" + requestId + ",\"error\":\"Record not found\"}")
            .build();
    }

    @POST
    @Path("/server-error")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Always server error endpoint (500)")
    public Response serverError(String body) {
        int requestId = requestCounter.incrementAndGet();
        LOG.errorf("Server error endpoint - Request #%d returning 500", requestId);
        return Response.status(500)
            .entity("{\"status\":\"error\",\"endpoint\":\"server-error\",\"requestId\":" + requestId + "}")
            .build();
    }
}
