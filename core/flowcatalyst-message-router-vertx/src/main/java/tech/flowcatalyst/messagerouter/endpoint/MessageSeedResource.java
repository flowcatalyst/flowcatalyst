package tech.flowcatalyst.messagerouter.endpoint;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.inject.Inject;
import jakarta.ws.rs.Consumes;
import jakarta.ws.rs.POST;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.SendMessageRequest;
import tech.flowcatalyst.messagerouter.config.QueueType;
import tech.flowcatalyst.messagerouter.embedded.EmbeddedQueuePublisher;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.security.Protected;

import java.util.Random;
import java.util.UUID;

@Path("/api/seed")
@Tag(name = "Message Seeding", description = "Endpoints for seeding test messages to queues")
@Protected("Message seeding endpoints requiring authentication")
public class MessageSeedResource {

    private static final Logger LOG = Logger.getLogger(MessageSeedResource.class);
    private final Random random = new Random();

    private static final String[] QUEUES = {
        "http://localhost:4566/000000000000/flow-catalyst-high-priority.fifo",
        "http://localhost:4566/000000000000/flow-catalyst-medium-priority.fifo",
        "http://localhost:4566/000000000000/flow-catalyst-low-priority.fifo"
    };

    private static final String[] POOL_CODES = {"POOL-HIGH", "POOL-MEDIUM", "POOL-LOW"};

    private static final String[] ENDPOINTS = {
        "http://localhost:8080/api/test/fast",
        "http://localhost:8080/api/test/slow",
        "http://localhost:8080/api/test/faulty",
        "http://localhost:8080/api/test/fail"
    };

    @ConfigProperty(name = "message-router.queue-type")
    QueueType queueType;

    @Inject
    SqsClient sqsClient;

    @Inject
    EmbeddedQueuePublisher embeddedQueuePublisher;

    @Inject
    ObjectMapper objectMapper;

    @POST
    @Path("/messages")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    @Operation(summary = "Seed messages to queues", description = "Send test messages to SQS queues for testing message routing")
    public Response seedMessages(SeedMessageRequest request) {

        try {
            // Use defaults if request is null
            if (request == null) {
                request = new SeedMessageRequest(10, "random", "random", "1of8");
            }

            int count = request.count();
            String queueParam = request.queue();
            String endpointParam = request.endpoint();
            String messageGroupMode = request.messageGroupMode();

            // Warn if seeding a lot of messages with localhost endpoint (potential deadlock)
            if (count > 100 && endpointParam != null &&
                (endpointParam.contains("localhost") || endpointParam.contains("127.0.0.1"))) {
                LOG.warnf("⚠️  Seeding %d messages to localhost endpoint - this may cause deadlock! " +
                         "Consider using a smaller count or an external endpoint.", count);
            }

            int successCount = 0;

            for (int i = 0; i < count; i++) {
                String queueUrl = selectQueue(queueParam);
                String poolCode = getPoolCodeForQueue(queueUrl);
                String targetEndpoint = selectEndpoint(endpointParam);
                String messageGroupId = selectMessageGroup(messageGroupMode, i);

                MessagePointer message = new MessagePointer(
                    UUID.randomUUID().toString(),
                    poolCode,
                    "test-token-" + UUID.randomUUID(),
                    tech.flowcatalyst.messagerouter.model.MediationType.HTTP,
                    targetEndpoint,
                    messageGroupId,  // Add messageGroupId for FIFO ordering
                    null  // batchId is populated by QueueManager during routing
                );

                String messageBody = objectMapper.writeValueAsString(message);

                // Publish to appropriate queue based on type
                switch (queueType) {
                    case SQS -> {
                        SendMessageRequest sqsRequest = SendMessageRequest.builder()
                            .queueUrl(queueUrl)
                            .messageBody(messageBody)
                            .messageGroupId(messageGroupId)
                            .messageDeduplicationId(message.id())
                            .build();
                        sqsClient.sendMessage(sqsRequest);
                    }
                    case EMBEDDED -> {
                        String queueName = getPoolCodeForQueue(queueUrl);
                        embeddedQueuePublisher.publishMessage(
                            message.id(),
                            messageGroupId,
                            message.id(), // Use message ID as dedup ID
                            messageBody
                        );
                    }
                    case ACTIVEMQ -> {
                        LOG.warnf("ActiveMQ message seeding not yet implemented");
                    }
                    case NATS -> {
                        LOG.warnf("NATS JetStream message seeding not yet implemented");
                    }
                }

                successCount++;
            }

            LOG.infof("Successfully sent %d messages to %s queue targeting %s endpoint",
                successCount, queueParam, endpointParam);

            return Response.ok()
                .entity(SeedMessageResponse.success(successCount, count))
                .build();

        } catch (Exception e) {
            LOG.error("Error seeding messages", e);
            return Response.status(Response.Status.INTERNAL_SERVER_ERROR)
                .entity(SeedMessageResponse.error(e.getMessage()))
                .build();
        }
    }

    private String selectQueue(String queueParam) {
        String lowerParam = queueParam.toLowerCase();

        // Check if it's already a full queue URL
        if (lowerParam.startsWith("http://") || lowerParam.startsWith("https://")) {
            return queueParam;
        }

        // Check if it matches a queue name pattern
        if (lowerParam.contains("high-priority") || lowerParam.contains("high")) {
            return QUEUES[0];
        }
        if (lowerParam.contains("medium-priority") || lowerParam.contains("medium")) {
            return QUEUES[1];
        }
        if (lowerParam.contains("low-priority") || lowerParam.contains("low")) {
            return QUEUES[2];
        }

        // Default to random
        return QUEUES[random.nextInt(QUEUES.length)];
    }

    private String getPoolCodeForQueue(String queueUrl) {
        if (queueUrl.contains("high-priority")) return POOL_CODES[0];
        if (queueUrl.contains("medium-priority")) return POOL_CODES[1];
        if (queueUrl.contains("low-priority")) return POOL_CODES[2];
        return POOL_CODES[0];
    }

    private String selectEndpoint(String endpointParam) {
        String lowerParam = endpointParam.toLowerCase();

        // Check if it's already a full URL
        if (lowerParam.startsWith("http://") || lowerParam.startsWith("https://")) {
            return endpointParam;
        }

        // Match endpoint types
        return switch (lowerParam) {
            case "fast" -> ENDPOINTS[0];
            case "slow" -> ENDPOINTS[1];
            case "faulty" -> ENDPOINTS[2];
            case "fail" -> ENDPOINTS[3];
            default -> ENDPOINTS[random.nextInt(ENDPOINTS.length)];
        };
    }

    private String getQueueName(String queueUrl) {
        String[] parts = queueUrl.split("/");
        return parts[parts.length - 1];
    }

    private String selectMessageGroup(String mode, int messageIndex) {
        return switch (mode.toLowerCase()) {
            case "unique" -> "msg-" + messageIndex; // Each message gets unique group (max parallelism)
            case "1of8" -> "group-" + random.nextInt(8); // Random selection from 8 groups
            case "single" -> "single-group"; // All messages in same group (strict FIFO ordering)
            default -> "group-" + random.nextInt(8); // Default to 1of8
        };
    }
}
