package tech.flowcatalyst.messagerouter.endpoint;

import com.fasterxml.jackson.annotation.JsonProperty;
import io.quarkus.scheduler.Scheduled;
import io.smallrye.common.annotation.RunOnVirtualThread;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.jetbrains.annotations.Async;

import java.time.temporal.ChronoUnit;

@Schema(description = "Request to seed test messages to queues")
public record SeedMessageRequest(

    @Schema(description = "Number of messages to send (WARNING: >100 to localhost may cause deadlock)",
            defaultValue = "10",
            examples = {"10", "50", "100", "1000"})
    @JsonProperty(defaultValue = "10")
    Integer count,

    @Schema(description = "Target queue: 'high', 'medium', 'low', 'random', or full queue name like 'flow-catalyst-high-priority.fifo'",
            defaultValue = "random",
            examples = {"high", "medium", "low", "random"})
    @JsonProperty(defaultValue = "random")
    String queue,

    @Schema(description = "Target endpoint: 'fast', 'slow', 'faulty', 'fail', 'random', or full URL like 'https://httpbin.org/post'",
            defaultValue = "random",
            examples = {"fast", "slow", "faulty", "fail", "random"})
    @JsonProperty(defaultValue = "random")
    String endpoint,

    @Schema(description = "Message group mode: 'unique' (each message unique group, max parallelism), '1of8' (random from 8 groups), 'single' (all messages same group, strict ordering)",
            defaultValue = "1of8",
            examples = {"unique", "1of8", "single"})
    @JsonProperty(defaultValue = "1of8")
    String messageGroupMode
) {

    public SeedMessageRequest {
        // Provide defaults for null values
        count = count != null ? count : 10;
        queue = queue != null && !queue.isBlank() ? queue : "random";
        endpoint = endpoint != null && !endpoint.isBlank() ? endpoint : "random";
        messageGroupMode = messageGroupMode != null && !messageGroupMode.isBlank() ? messageGroupMode : "1of8";
    }
}
