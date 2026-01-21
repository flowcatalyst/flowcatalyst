package tech.flowcatalyst.benchmark;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.quarkus.picocli.runtime.annotations.TopCommand;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import picocli.CommandLine;
import picocli.CommandLine.Command;
import picocli.CommandLine.Option;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.SendMessageRequest;
import software.amazon.awssdk.services.sqs.model.SendMessageBatchRequest;
import software.amazon.awssdk.services.sqs.model.SendMessageBatchRequestEntry;

import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.ThreadLocalRandom;
import java.util.concurrent.atomic.AtomicInteger;

@TopCommand
@Command(name = "benchmark", mixinStandardHelpOptions = true, version = "1.0",
    description = "FlowCatalyst Message Router Benchmark Tool")
public class BenchmarkCommand implements Callable<Integer> {

    private static final Logger LOG = Logger.getLogger(BenchmarkCommand.class);

    @Inject
    SqsClient sqsClient;

    @Inject
    ObjectMapper objectMapper;

    @Option(names = {"-q", "--queue-url"}, required = true,
        description = "SQS Queue URL")
    String queueUrl;

    @Option(names = {"-n", "--num-messages"}, defaultValue = "1000",
        description = "Total number of messages to send (default: 1000)")
    int numMessages;

    @Option(names = {"-g", "--num-groups"}, defaultValue = "10",
        description = "Number of unique message groups (default: 10)")
    int numGroups;

    @Option(names = {"-p", "--pool-code"}, defaultValue = "BENCHMARK-POOL",
        description = "Pool code for message routing (default: BENCHMARK-POOL)")
    String poolCode;

    @Option(names = {"-t", "--target-endpoint"}, required = true,
        description = "Target endpoint URL (e.g., http://localhost:8080/api/benchmark/process)")
    String targetEndpoint;

    @Option(names = {"-a", "--auth-token"},
        description = "Optional auth token for the target endpoint")
    String authToken;

    @Option(names = {"-b", "--batch-size"}, defaultValue = "10",
        description = "Batch size for SQS SendMessageBatch (1-10, default: 10)")
    int batchSize;

    @Option(names = {"-r", "--rate-limit"},
        description = "Optional rate limit (messages per second). If not set, sends as fast as possible")
    Integer rateLimit;

    @Option(names = {"--dry-run"}, defaultValue = "false",
        description = "Print sample messages without sending (default: false)")
    boolean dryRun;

    @Override
    public Integer call() throws Exception {
        LOG.infof("=== FlowCatalyst Benchmark ===");
        LOG.infof("Queue URL: %s", queueUrl);
        LOG.infof("Total Messages: %d", numMessages);
        LOG.infof("Message Groups: %d", numGroups);
        LOG.infof("Pool Code: %s", poolCode);
        LOG.infof("Target Endpoint: %s", targetEndpoint);
        LOG.infof("Batch Size: %d", batchSize);
        LOG.infof("Rate Limit: %s", rateLimit != null ? rateLimit + " msg/s" : "unlimited");
        LOG.infof("Dry Run: %s", dryRun);
        LOG.info("==============================");

        if (batchSize < 1 || batchSize > 10) {
            LOG.error("Batch size must be between 1 and 10");
            return 1;
        }

        // Generate message groups
        List<String> messageGroups = new ArrayList<>();
        for (int i = 0; i < numGroups; i++) {
            messageGroups.add("group-" + i);
        }

        if (dryRun) {
            LOG.info("DRY RUN: Printing sample messages...");
            printSampleMessages(messageGroups);
            return 0;
        }

        // Send messages
        long startTime = System.currentTimeMillis();
        AtomicInteger sent = new AtomicInteger(0);
        AtomicInteger failed = new AtomicInteger(0);

        List<SendMessageBatchRequestEntry> batch = new ArrayList<>();
        int batchId = 0;

        long lastSendTime = System.currentTimeMillis();
        int messagesThisSecond = 0;

        for (int i = 0; i < numMessages; i++) {
            // Select message group (round-robin for even distribution)
            String messageGroupId = messageGroups.get(i % messageGroups.size());

            // Create message pointer
            MessagePointer pointer = new MessagePointer(
                UUID.randomUUID().toString(),
                poolCode,
                authToken,
                "HTTP",
                targetEndpoint,
                messageGroupId
            );

            String messageBody = objectMapper.writeValueAsString(pointer);

            // Add to batch
            batch.add(SendMessageBatchRequestEntry.builder()
                .id(String.valueOf(i))
                .messageBody(messageBody)
                .messageGroupId(messageGroupId)  // SQS FIFO attribute
                .messageDeduplicationId(UUID.randomUUID().toString())  // SQS FIFO deduplication
                .build());

            // Send batch when full or last message
            if (batch.size() >= batchSize || i == numMessages - 1) {
                try {
                    // Rate limiting
                    if (rateLimit != null) {
                        messagesThisSecond += batch.size();
                        if (messagesThisSecond >= rateLimit) {
                            long elapsed = System.currentTimeMillis() - lastSendTime;
                            if (elapsed < 1000) {
                                Thread.sleep(1000 - elapsed);
                            }
                            lastSendTime = System.currentTimeMillis();
                            messagesThisSecond = 0;
                        }
                    }

                    SendMessageBatchRequest request = SendMessageBatchRequest.builder()
                        .queueUrl(queueUrl)
                        .entries(batch)
                        .build();

                    sqsClient.sendMessageBatch(request);
                    sent.addAndGet(batch.size());

                    batchId++;
                    if (batchId % 10 == 0) {
                        LOG.infof("Sent %d/%d messages (%.1f%%)...",
                            sent.get(), numMessages, (sent.get() * 100.0 / numMessages));
                    }

                } catch (Exception e) {
                    failed.addAndGet(batch.size());
                    LOG.errorf(e, "Failed to send batch %d", batchId);
                }

                batch.clear();
            }
        }

        long endTime = System.currentTimeMillis();
        long durationMs = endTime - startTime;
        double durationSec = durationMs / 1000.0;
        double throughput = sent.get() / durationSec;

        LOG.info("==============================");
        LOG.infof("=== Benchmark Complete ===");
        LOG.infof("Total Messages Sent: %d", sent.get());
        LOG.infof("Failed: %d", failed.get());
        LOG.infof("Duration: %.2f seconds", durationSec);
        LOG.infof("Throughput: %.2f msg/s", throughput);
        LOG.info("==============================");

        return 0;
    }

    private void printSampleMessages(List<String> messageGroups) {
        try {
            for (int i = 0; i < Math.min(5, numMessages); i++) {
                String messageGroupId = messageGroups.get(i % messageGroups.size());

                MessagePointer pointer = new MessagePointer(
                    UUID.randomUUID().toString(),
                    poolCode,
                    authToken,
                    "HTTP",
                    targetEndpoint,
                    messageGroupId
                );

                String messageBody = objectMapper.writerWithDefaultPrettyPrinter()
                    .writeValueAsString(pointer);

                LOG.infof("Sample Message %d (Group: %s):\n%s\n", i + 1, messageGroupId, messageBody);
            }
        } catch (Exception e) {
            LOG.errorf(e, "Failed to print sample messages");
        }
    }

    /**
     * Message pointer model matching the message router's expected format
     */
    record MessagePointer(
        String id,
        String poolCode,
        String authToken,
        String mediationType,
        String mediationTarget,
        String messageGroupId
    ) {}
}
