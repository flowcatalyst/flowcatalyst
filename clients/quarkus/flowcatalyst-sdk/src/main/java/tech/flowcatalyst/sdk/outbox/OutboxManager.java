package tech.flowcatalyst.sdk.outbox;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import tech.flowcatalyst.sdk.enums.MessageType;
import tech.flowcatalyst.sdk.exception.OutboxException;
import tech.flowcatalyst.sdk.outbox.driver.OutboxDriver;
import tech.flowcatalyst.sdk.outbox.dto.CreateDispatchJobDto;
import tech.flowcatalyst.sdk.outbox.dto.CreateEventDto;
import tech.flowcatalyst.sdk.support.TsidGenerator;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Manager for creating outbox messages (events and dispatch jobs).
 *
 * <p>The outbox pattern ensures reliable event delivery by persisting messages
 * to the database before they are dispatched. A separate process then picks
 * up these messages and delivers them.
 */
public class OutboxManager {

    private final OutboxDriver driver;
    private final String tenantId;
    private final String defaultPartition;
    private final ObjectMapper objectMapper;

    public OutboxManager(OutboxDriver driver, String tenantId) {
        this(driver, tenantId, "default");
    }

    public OutboxManager(OutboxDriver driver, String tenantId, String defaultPartition) {
        this.driver = driver;
        this.tenantId = tenantId;
        this.defaultPartition = defaultPartition;
        this.objectMapper = new ObjectMapper();
    }

    /**
     * Create an event in the outbox.
     *
     * @param event The event to create
     * @return The generated message ID (TSID)
     */
    public String createEvent(CreateEventDto event) {
        ensureTenantId();

        String id = TsidGenerator.generate();
        String payload = toJson(event.toPayload());

        Map<String, Object> message = new HashMap<>();
        message.put("id", id);
        message.put("tenant_id", tenantId);
        message.put("partition_id", event.partitionId() != null ? event.partitionId() : defaultPartition);
        message.put("type", MessageType.EVENT.name());
        message.put("payload", payload);
        message.put("payload_size", payload.length());
        message.put("status", "PENDING");
        message.put("created_at", Instant.now());
        message.put("headers", event.headers() != null && !event.headers().isEmpty() ? event.headers() : null);

        driver.insert(message);

        return id;
    }

    /**
     * Create a dispatch job in the outbox.
     *
     * @param job The dispatch job to create
     * @return The generated message ID (TSID)
     */
    public String createDispatchJob(CreateDispatchJobDto job) {
        ensureTenantId();

        String id = TsidGenerator.generate();
        String payload = toJson(job.toPayload());

        Map<String, Object> message = new HashMap<>();
        message.put("id", id);
        message.put("tenant_id", tenantId);
        message.put("partition_id", job.partitionId() != null ? job.partitionId() : defaultPartition);
        message.put("type", MessageType.DISPATCH_JOB.name());
        message.put("payload", payload);
        message.put("payload_size", payload.length());
        message.put("status", "PENDING");
        message.put("created_at", Instant.now());
        message.put("headers", null);

        driver.insert(message);

        return id;
    }

    /**
     * Create multiple events in the outbox (batch).
     *
     * @param events The events to create
     * @return The generated message IDs (TSIDs)
     */
    public List<String> createEvents(List<CreateEventDto> events) {
        if (events == null || events.isEmpty()) {
            return List.of();
        }

        ensureTenantId();

        List<Map<String, Object>> messages = new ArrayList<>();
        List<String> ids = new ArrayList<>();

        for (CreateEventDto event : events) {
            String id = TsidGenerator.generate();
            ids.add(id);

            String payload = toJson(event.toPayload());

            Map<String, Object> message = new HashMap<>();
            message.put("id", id);
            message.put("tenant_id", tenantId);
            message.put("partition_id", event.partitionId() != null ? event.partitionId() : defaultPartition);
            message.put("type", MessageType.EVENT.name());
            message.put("payload", payload);
            message.put("payload_size", payload.length());
            message.put("status", "PENDING");
            message.put("created_at", Instant.now());
            message.put("headers", event.headers() != null && !event.headers().isEmpty() ? event.headers() : null);

            messages.add(message);
        }

        driver.insertBatch(messages);

        return ids;
    }

    /**
     * Create multiple dispatch jobs in the outbox (batch).
     *
     * @param jobs The dispatch jobs to create
     * @return The generated message IDs (TSIDs)
     */
    public List<String> createDispatchJobs(List<CreateDispatchJobDto> jobs) {
        if (jobs == null || jobs.isEmpty()) {
            return List.of();
        }

        ensureTenantId();

        List<Map<String, Object>> messages = new ArrayList<>();
        List<String> ids = new ArrayList<>();

        for (CreateDispatchJobDto job : jobs) {
            String id = TsidGenerator.generate();
            ids.add(id);

            String payload = toJson(job.toPayload());

            Map<String, Object> message = new HashMap<>();
            message.put("id", id);
            message.put("tenant_id", tenantId);
            message.put("partition_id", job.partitionId() != null ? job.partitionId() : defaultPartition);
            message.put("type", MessageType.DISPATCH_JOB.name());
            message.put("payload", payload);
            message.put("payload_size", payload.length());
            message.put("status", "PENDING");
            message.put("created_at", Instant.now());
            message.put("headers", null);

            messages.add(message);
        }

        driver.insertBatch(messages);

        return ids;
    }

    /**
     * Get the underlying driver.
     */
    public OutboxDriver driver() {
        return driver;
    }

    private void ensureTenantId() {
        if (tenantId == null || tenantId.isEmpty()) {
            throw OutboxException.missingTenantId();
        }
    }

    private String toJson(Map<String, Object> map) {
        try {
            return objectMapper.writeValueAsString(map);
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize payload", e);
        }
    }
}
