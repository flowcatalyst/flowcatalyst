package io.flowcatalyst.sdk.outbox;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.ObjectMapper;
import io.flowcatalyst.sdk.http.Json;
import io.flowcatalyst.sdk.tsid.Tsid;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Manages outbox message creation for the transactional outbox pattern.
 *
 * <pre>{@code
 * var outbox = new OutboxManager(driver, "clt_0HZXEQ5Y8JY5Z");
 *
 * // Single event
 * String eventId = outbox.createEvent(
 *         CreateEventDto.create("orders:sales:order:created", Map.of("orderId", "123")));
 *
 * // Batch of dispatch jobs inside the caller's transaction
 * List<String> jobIds = outbox.createDispatchJobs(List.of(job1, job2), tx);
 * }</pre>
 */
public final class OutboxManager {

    private final OutboxDriver driver;
    private final String clientId;
    private final InstantSource clock;
    private final ObjectMapper mapper = Json.newMapper();

    /**
     * @param clientId the FlowCatalyst client (tenant) TSID that owns the
     *                 outbox rows, e.g. {@code clt_0HZXEQ5Y8JY5Z}
     */
    public OutboxManager(OutboxDriver driver, String clientId) {
        this(driver, clientId, InstantSource.system());
    }

    /**
     * As {@link #OutboxManager(OutboxDriver, String)}, with the clock that dates each row
     * ({@code created_at}, {@code updated_at}) and an audit entry without its own
     * {@code performedAt}.
     */
    public OutboxManager(OutboxDriver driver, String clientId, InstantSource clock) {
        this.driver = driver;
        this.clientId = clientId;
        this.clock = clock;
    }

    public String createEvent(CreateEventDto event) {
        return createEvent(event, null);
    }

    /**
     * Create a single event in the outbox. Returns the generated TSID.
     *
     * @param tx optional transaction handle. If provided, the row joins that
     *           transaction so the outbox write is atomic with the caller's
     *           business writes — pass the same handle to both.
     */
    public String createEvent(CreateEventDto event, Object tx) {
        ensureClientId();
        String id = Tsid.generate();
        driver.insert(buildMessage(id, OutboxMessage.MessageType.EVENT,
                serialize(event.toPayload(mapper, id)), event.messageGroup(),
                nullIfEmpty(event.headers()), clock.instant()), tx);
        return id;
    }

    public List<String> createEvents(List<CreateEventDto> events) {
        return createEvents(events, null);
    }

    /** Create multiple events in the outbox (batch). Returns the generated TSIDs. */
    public List<String> createEvents(List<CreateEventDto> events, Object tx) {
        if (events.isEmpty()) return List.of();
        ensureClientId();
        Instant now = clock.instant();
        List<String> ids = new ArrayList<>(events.size());
        List<OutboxMessage> messages = new ArrayList<>(events.size());
        for (CreateEventDto event : events) {
            String id = Tsid.generate();
            ids.add(id);
            messages.add(buildMessage(id, OutboxMessage.MessageType.EVENT,
                    serialize(event.toPayload(mapper, id)), event.messageGroup(),
                    nullIfEmpty(event.headers()), now));
        }
        driver.insertBatch(messages, tx);
        return ids;
    }

    public String createDispatchJob(CreateDispatchJobDto job) {
        return createDispatchJob(job, null);
    }

    /**
     * Create a single dispatch job in the outbox. Returns the job id, which is also the row id:
     * the one given with {@link CreateDispatchJobDto#withId}, or a generated TSID.
     *
     * @throws IllegalArgumentException when {@link CreateDispatchJobDto#validate()} refuses the
     *     job (nothing is written)
     */
    public String createDispatchJob(CreateDispatchJobDto job, Object tx) {
        ensureClientId();
        String id = job.id() != null ? job.id() : Tsid.generate();
        driver.insert(buildMessage(id, OutboxMessage.MessageType.DISPATCH_JOB,
                serialize(job.toPayload(id)), job.messageGroup(), null, clock.instant()), tx);
        return id;
    }

    public List<String> createDispatchJobs(List<CreateDispatchJobDto> jobs) {
        return createDispatchJobs(jobs, null);
    }

    /**
     * Create multiple dispatch jobs in the outbox (batch). Returns the job ids (see
     * {@link #createDispatchJob(CreateDispatchJobDto, Object)}); every job is validated before
     * any row is written.
     */
    public List<String> createDispatchJobs(List<CreateDispatchJobDto> jobs, Object tx) {
        if (jobs.isEmpty()) return List.of();
        ensureClientId();
        Instant now = clock.instant();
        List<String> ids = new ArrayList<>(jobs.size());
        List<OutboxMessage> messages = new ArrayList<>(jobs.size());
        for (CreateDispatchJobDto job : jobs) {
            String id = job.id() != null ? job.id() : Tsid.generate();
            ids.add(id);
            messages.add(buildMessage(id, OutboxMessage.MessageType.DISPATCH_JOB,
                    serialize(job.toPayload(id)), job.messageGroup(), null, now));
        }
        driver.insertBatch(messages, tx);
        return ids;
    }

    public String createAuditLog(CreateAuditLogDto auditLog) {
        return createAuditLog(auditLog, null);
    }

    /** Create a single audit log in the outbox. Returns the generated TSID. */
    public String createAuditLog(CreateAuditLogDto auditLog, Object tx) {
        ensureClientId();
        String id = Tsid.generate();
        Instant now = clock.instant();
        driver.insert(buildMessage(id, OutboxMessage.MessageType.AUDIT_LOG,
                serialize(auditLog.toPayload(mapper, now)), null,
                nullIfEmpty(auditLog.headers()), now), tx);
        return id;
    }

    public List<String> createAuditLogs(List<CreateAuditLogDto> auditLogs) {
        return createAuditLogs(auditLogs, null);
    }

    /** Create multiple audit logs in the outbox (batch). Returns the generated TSIDs. */
    public List<String> createAuditLogs(List<CreateAuditLogDto> auditLogs, Object tx) {
        if (auditLogs.isEmpty()) return List.of();
        ensureClientId();
        Instant now = clock.instant();
        List<String> ids = new ArrayList<>(auditLogs.size());
        List<OutboxMessage> messages = new ArrayList<>(auditLogs.size());
        for (CreateAuditLogDto auditLog : auditLogs) {
            String id = Tsid.generate();
            ids.add(id);
            messages.add(buildMessage(id, OutboxMessage.MessageType.AUDIT_LOG,
                    serialize(auditLog.toPayload(mapper, now)), null,
                    nullIfEmpty(auditLog.headers()), now));
        }
        driver.insertBatch(messages, tx);
        return ids;
    }

    public OutboxDriver driver() {
        return driver;
    }

    private OutboxMessage buildMessage(
            String id,
            OutboxMessage.MessageType type,
            String payload,
            String messageGroup,
            Map<String, String> headers,
            Instant now) {
        return new OutboxMessage(
                id,
                type,
                messageGroup,
                payload,
                OutboxMessage.Status.PENDING,
                now,
                now,
                clientId,
                payload.getBytes(StandardCharsets.UTF_8).length,
                headers);
    }

    private String serialize(Map<String, Object> payload) {
        try {
            return mapper.writeValueAsString(payload);
        } catch (JacksonException e) {
            throw new IllegalArgumentException("Outbox payload is not serializable to JSON", e);
        }
    }

    private static Map<String, String> nullIfEmpty(Map<String, String> headers) {
        return headers == null || headers.isEmpty() ? null : headers;
    }

    private void ensureClientId() {
        if (clientId == null || clientId.isEmpty()) {
            throw new IllegalStateException(
                    "OutboxManager: clientId is required. Provide a valid client ID when "
                            + "constructing the OutboxManager.");
        }
    }
}
