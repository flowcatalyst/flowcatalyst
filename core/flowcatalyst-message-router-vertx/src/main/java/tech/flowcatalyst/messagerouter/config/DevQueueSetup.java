package tech.flowcatalyst.messagerouter.config;

import io.quarkus.arc.profile.IfBuildProfile;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.*;

import java.util.HashMap;
import java.util.Map;

@ApplicationScoped
@IfBuildProfile("dev")
public class DevQueueSetup {

    private static final Logger LOG = Logger.getLogger(DevQueueSetup.class);

    @Inject
    SqsClient sqsClient;

    private static final String[] QUEUE_NAMES = {
        "flow-catalyst-high-priority.fifo",
        "flow-catalyst-medium-priority.fifo",
        "flow-catalyst-low-priority.fifo",
        "flow-catalyst-dispatch.fifo"
    };

    void onStart(@Observes StartupEvent event) {
        LOG.info("Dev mode: Checking/creating SQS queues in LocalStack");

        for (String queueName : QUEUE_NAMES) {
            try {
                // Try to get queue URL - if it exists, this will succeed
                GetQueueUrlRequest getUrlRequest = GetQueueUrlRequest.builder()
                    .queueName(queueName)
                    .build();

                GetQueueUrlResponse urlResponse = sqsClient.getQueueUrl(getUrlRequest);
                LOG.infof("Queue already exists: %s -> %s", queueName, urlResponse.queueUrl());

            } catch (QueueDoesNotExistException e) {
                // Queue doesn't exist, create it
                LOG.infof("Creating FIFO queue: %s", queueName);

                Map<QueueAttributeName, String> attributes = new HashMap<>();
                attributes.put(QueueAttributeName.FIFO_QUEUE, "true");
                attributes.put(QueueAttributeName.CONTENT_BASED_DEDUPLICATION, "false");
                attributes.put(QueueAttributeName.DEDUPLICATION_SCOPE, "queue");
                attributes.put(QueueAttributeName.FIFO_THROUGHPUT_LIMIT, "perQueue");

                CreateQueueRequest createRequest = CreateQueueRequest.builder()
                    .queueName(queueName)
                    .attributes(attributes)
                    .build();

                CreateQueueResponse createResponse = sqsClient.createQueue(createRequest);
                LOG.infof("Created queue: %s -> %s", queueName, createResponse.queueUrl());

            } catch (Exception e) {
                LOG.errorf(e, "Error checking/creating queue: %s", queueName);
            }
        }

        LOG.info("Dev mode: Queue setup complete");
    }
}
