package io.flowcatalyst.sdk.outbox;

import io.flowcatalyst.sdk.http.Json;
import java.net.URI;
import java.net.URISyntaxException;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.regex.Pattern;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.ObjectMapper;

/**
 * A dispatch job: one item of the platform's {@code POST /api/dispatch-jobs/batch}, written to
 * the outbox ({@link OutboxManager#createDispatchJob}) or sent directly
 * ({@code client.dispatchJobs().createBatch(...)}). Immutable builder: every {@code with*}
 * method returns a new instance.
 *
 * <p>Two entry points:
 *
 * <ul>
 *   <li>{@link #of(String, String)} (0.0.13): the framework's write-path contract 7a defaults,
 *       the shape the conformance vectors pin: kind {@code TASK}, source the code's application
 *       segment, payload {@code {}} as {@code application/json}, {@code dataOnly} false, and
 *       {@code sequence}, {@code timeoutSeconds}, {@code maxRetries}, {@code mode} left to the
 *       platform (99, 30, 3, {@code NEXT_ON_ERROR}).
 *       <pre>{@code
 * var job = CreateDispatchJobDto.of("ingest:orders:record", "https://api.example.com/records")
 *         .withClientCode("acme")
 *         .withPayload(csvLine, "text/csv")
 *         .withMessageGroup("file-123")
 *         .withSequence(7);
 * }</pre>
 *   <li>{@link #create(String, String, String, String, String)}: unchanged defaults from 0.0.12
 *       ({@code dataOnly} true, {@code timeoutSeconds} 30, {@code maxRetries} 5, kind left to the
 *       platform, which takes {@code EVENT}), so existing callers deliver exactly as before.
 * </ul>
 *
 * <p>Every job is checked by {@link #validate()} before it is written or sent, so a row the
 * platform would refuse terminally (a whole-batch {@code 400}) is never stored. The payload is
 * the batch item: {@code metadata} is the platform's array of {@code {"key","value"}} pairs (an
 * object fails the whole batch), a client is named by its platform id
 * ({@link #withPlatformClientId}) or by its identifier ({@link #withClientCode}, resolved by the
 * platform: an unknown code refuses the whole batch, {@code 400} for an anchor and {@code 403}
 * otherwise), and the members the batch endpoint ignores are never written.
 */
public final class CreateDispatchJobDto {

    /**
     * Ordering behavior within a message group.
     * IMMEDIATE: no ordering, jobs dispatch concurrently.
     * NEXT_ON_ERROR: FIFO per group; a failed job is retried later but the group moves on
     * (<strong>the platform default when {@code mode} is unset</strong>).
     * BLOCK_ON_ERROR: strict FIFO per group; a failed job blocks the group until resolved.
     *
     * <p>The default is NEXT_ON_ERROR because the two failure modes are not
     * symmetric: wanting concurrency and getting ordering costs throughput,
     * which is visible and cheap to fix, while needing ordering and silently
     * getting none is invisible and lands in the target's data.
     */
    public enum DispatchMode {
        IMMEDIATE,
        NEXT_ON_ERROR,
        BLOCK_ON_ERROR
    }

    /**
     * What the job is. {@code TASK}: a command the application creates for one target (the
     * default of {@link #of}). {@code EVENT}: a job fanned out from an event (the platform's
     * default when no kind is sent, as {@link #create} does).
     */
    public enum Kind {
        EVENT,
        TASK
    }

    private static final Pattern CODE = Pattern.compile("^[a-z][a-z0-9-]*:.+$", Pattern.DOTALL);
    private static final Pattern PLATFORM_ID = Pattern.compile("^[0-9A-Za-z_-]{1,17}$");
    private static final ObjectMapper MAPPER = Json.newMapper();

    private String id;
    private Kind kind;
    private String source;
    private String code;
    private String targetUrl;
    private String payload;
    private String payloadContentType = "application/json";
    private String dispatchPoolId;
    private String subject;
    private String correlationId;
    private String eventId;
    private String platformClientId;
    private String clientCode;
    private String subscriptionId;
    private String serviceAccountId;
    private Map<String, String> metadata = Map.of();
    private Map<String, String> headers = Map.of();
    private boolean dataOnly;
    private String messageGroup;
    private DispatchMode mode;
    private Integer sequence;
    private Integer timeoutSeconds;
    private Integer maxRetries;
    private String retryStrategy;
    private Instant scheduledFor;
    private Instant expiresAt;
    private String idempotencyKey;
    private String externalId;
    private String connectionId;
    private String queue;
    private String descriptor;

    private CreateDispatchJobDto() {}

    private CreateDispatchJobDto copy() {
        CreateDispatchJobDto c = new CreateDispatchJobDto();
        c.id = id;
        c.kind = kind;
        c.source = source;
        c.code = code;
        c.targetUrl = targetUrl;
        c.payload = payload;
        c.payloadContentType = payloadContentType;
        c.dispatchPoolId = dispatchPoolId;
        c.subject = subject;
        c.correlationId = correlationId;
        c.eventId = eventId;
        c.platformClientId = platformClientId;
        c.clientCode = clientCode;
        c.subscriptionId = subscriptionId;
        c.serviceAccountId = serviceAccountId;
        c.metadata = metadata;
        c.headers = headers;
        c.dataOnly = dataOnly;
        c.messageGroup = messageGroup;
        c.mode = mode;
        c.sequence = sequence;
        c.timeoutSeconds = timeoutSeconds;
        c.maxRetries = maxRetries;
        c.retryStrategy = retryStrategy;
        c.scheduledFor = scheduledFor;
        c.expiresAt = expiresAt;
        c.idempotencyKey = idempotencyKey;
        c.externalId = externalId;
        c.connectionId = connectionId;
        c.queue = queue;
        c.descriptor = descriptor;
        return c;
    }

    /**
     * A job for {@code code} delivered to {@code targetUrl}, with the platform's defaults (class
     * doc). Nothing is checked until {@link #validate()} (which writing and sending call).
     *
     * @param code {@code {application}:{rest}}, the application segment {@code [a-z][a-z0-9-]*}
     *     (the platform confines an application-scoped caller by it), at most 255 characters
     * @param targetUrl absolute {@code http(s)} URL with a host and no user info
     */
    public static CreateDispatchJobDto of(String code, String targetUrl) {
        CreateDispatchJobDto dto = new CreateDispatchJobDto();
        dto.code = code;
        dto.targetUrl = targetUrl;
        dto.kind = Kind.TASK;
        dto.dataOnly = false;
        return dto;
    }

    /**
     * The 0.0.12 entry point, unchanged defaults: {@code dataOnly} true, {@code timeoutSeconds}
     * 30, {@code maxRetries} 5, no kind (the platform's {@code EVENT}). A null {@code source}
     * now defaults to the code's application segment and a null {@code payload} to {@code {}}
     * (both are {@code NOT NULL} on the platform, so a job without them could never be stored).
     *
     * @param code fully qualified {@code application:subdomain:aggregate:action}
     *             code — the platform facets on its segments and resolves
     *             delivery signing credentials from the application segment;
     *             bare codes are rejected
     * @param payload the payload string (serialize objects to JSON first)
     * @param dispatchPoolId the platform id of the pool, or null for the platform's default pool
     */
    public static CreateDispatchJobDto create(
            String source, String code, String targetUrl, String payload, String dispatchPoolId) {
        QualifiedCode.assertQualified(code, "Dispatch job code");
        CreateDispatchJobDto dto = new CreateDispatchJobDto();
        dto.source = source;
        dto.code = code;
        dto.targetUrl = targetUrl;
        dto.payload = payload;
        dto.dispatchPoolId = dispatchPoolId;
        dto.dataOnly = true;
        dto.timeoutSeconds = 30;
        dto.maxRetries = 5;
        return dto;
    }

    /**
     * The job id, {@code [0-9A-Za-z_-]{1,17}}. In the outbox the row id is the job id; when
     * unset the outbox generates a 13-character TSID for both. Because the id is in the payload,
     * a batch the outbox processor sends again creates a job with the same id, so a receiver
     * that runs a job once per id runs it once.
     */
    public CreateDispatchJobDto withId(String id) {
        CreateDispatchJobDto c = copy();
        c.id = id;
        return c;
    }

    public CreateDispatchJobDto withKind(Kind kind) {
        CreateDispatchJobDto c = copy();
        c.kind = kind;
        return c;
    }

    /** 1 to 255 characters; when unset, the code's application segment. */
    public CreateDispatchJobDto withSource(String source) {
        CreateDispatchJobDto c = copy();
        c.source = source;
        return c;
    }

    /** The delivered body and its content type (1 to 100 characters). */
    public CreateDispatchJobDto withPayload(String payload, String contentType) {
        CreateDispatchJobDto c = copy();
        c.payload = payload;
        c.payloadContentType = contentType;
        return c;
    }

    /** The delivered body: {@code value} as JSON, {@code application/json}. */
    public CreateDispatchJobDto withJsonPayload(Object value) {
        try {
            return withPayload(MAPPER.writeValueAsString(value), "application/json");
        } catch (JacksonException e) {
            throw new IllegalArgumentException("Dispatch job payload is not serializable to JSON", e);
        }
    }

    public CreateDispatchJobDto withSubject(String subject) {
        CreateDispatchJobDto c = copy();
        c.subject = subject;
        return c;
    }

    public CreateDispatchJobDto withCorrelationId(String correlationId) {
        CreateDispatchJobDto c = copy();
        c.correlationId = correlationId;
        return c;
    }

    /** The platform id of the event the job was made for. */
    public CreateDispatchJobDto withEventId(String eventId) {
        CreateDispatchJobDto c = copy();
        c.eventId = eventId;
        return c;
    }

    /**
     * The client by its platform id (the batch item's {@code clientId}, at most 17 characters).
     * Set this or {@link #withClientCode}, not both. Without either the job has no client.
     */
    public CreateDispatchJobDto withPlatformClientId(String platformClientId) {
        CreateDispatchJobDto c = copy();
        c.platformClientId = platformClientId;
        return c;
    }

    /**
     * The client by its identifier (slug), which the platform resolves at ingest. A code naming
     * a client the caller does not reach is {@code 403}; one naming no client is {@code 400}
     * for an anchor and {@code 403} otherwise; either refuses the whole batch. Trimmed when
     * written.
     */
    public CreateDispatchJobDto withClientCode(String clientCode) {
        CreateDispatchJobDto c = copy();
        c.clientCode = clientCode;
        return c;
    }

    public CreateDispatchJobDto withSubscriptionId(String subscriptionId) {
        CreateDispatchJobDto c = copy();
        c.subscriptionId = subscriptionId;
        return c;
    }

    public CreateDispatchJobDto withServiceAccountId(String serviceAccountId) {
        CreateDispatchJobDto c = copy();
        c.serviceAccountId = serviceAccountId;
        return c;
    }

    /** The platform id of the dispatch pool; unset uses the platform's default pool. */
    public CreateDispatchJobDto withDispatchPoolId(String dispatchPoolId) {
        CreateDispatchJobDto c = copy();
        c.dispatchPoolId = dispatchPoolId;
        return c;
    }

    /** Append one metadata pair (an existing key keeps its place and takes the new value). */
    public CreateDispatchJobDto withMetadata(String key, String value) {
        CreateDispatchJobDto c = copy();
        Map<String, String> merged = new LinkedHashMap<>(metadata);
        merged.put(key, value);
        c.metadata = Collections.unmodifiableMap(merged);
        return c;
    }

    /** Merge additional metadata (existing keys are overwritten), keeping insertion order. */
    public CreateDispatchJobDto withMetadata(Map<String, String> extra) {
        CreateDispatchJobDto c = copy();
        Map<String, String> merged = new LinkedHashMap<>(metadata);
        merged.putAll(extra);
        c.metadata = Collections.unmodifiableMap(merged);
        return c;
    }

    /**
     * Not sent: the dispatch-jobs batch endpoint has no {@code headers}.
     *
     * @deprecated ignored by the platform; kept so 0.0.12 code compiles
     */
    @Deprecated
    public CreateDispatchJobDto withHeaders(Map<String, String> extra) {
        CreateDispatchJobDto c = copy();
        Map<String, String> merged = new LinkedHashMap<>(headers);
        merged.putAll(extra);
        c.headers = Collections.unmodifiableMap(merged);
        return c;
    }

    public CreateDispatchJobDto withPayloadContentType(String payloadContentType) {
        CreateDispatchJobDto c = copy();
        c.payloadContentType = payloadContentType;
        return c;
    }

    /** True: the target receives the payload alone; false: the platform's delivery envelope. */
    public CreateDispatchJobDto withDataOnly(boolean dataOnly) {
        CreateDispatchJobDto c = copy();
        c.dataOnly = dataOnly;
        return c;
    }

    public CreateDispatchJobDto withMessageGroup(String messageGroup) {
        CreateDispatchJobDto c = copy();
        c.messageGroup = messageGroup;
        return c;
    }

    /** Ordering behavior within the message group; unset defaults to NEXT_ON_ERROR on the platform. */
    public CreateDispatchJobDto withMode(DispatchMode mode) {
        CreateDispatchJobDto c = copy();
        c.mode = mode;
        return c;
    }

    /** Order within the message group, at least 1 (the platform turns 0 into its default, 99). */
    public CreateDispatchJobDto withSequence(int sequence) {
        CreateDispatchJobDto c = copy();
        c.sequence = sequence;
        return c;
    }

    /** At least 1 (the platform turns 0 into its default, 30). */
    public CreateDispatchJobDto withTimeoutSeconds(int timeoutSeconds) {
        CreateDispatchJobDto c = copy();
        c.timeoutSeconds = timeoutSeconds;
        return c;
    }

    /** At least 1 (the platform turns 0 into its default, 3). */
    public CreateDispatchJobDto withMaxRetries(int maxRetries) {
        CreateDispatchJobDto c = copy();
        c.maxRetries = maxRetries;
        return c;
    }

    /**
     * Not sent: the batch endpoint always uses exponential backoff.
     *
     * @deprecated ignored by the platform; kept so 0.0.12 code compiles
     */
    @Deprecated
    public CreateDispatchJobDto withRetryStrategy(String retryStrategy) {
        CreateDispatchJobDto c = copy();
        c.retryStrategy = retryStrategy;
        return c;
    }

    /**
     * Not sent: the batch endpoint has no {@code scheduledFor}.
     *
     * @deprecated ignored by the platform; kept so 0.0.12 code compiles
     */
    @Deprecated
    public CreateDispatchJobDto withScheduledFor(Instant scheduledFor) {
        CreateDispatchJobDto c = copy();
        c.scheduledFor = scheduledFor;
        return c;
    }

    /**
     * Not sent: the batch endpoint has no {@code expiresAt}.
     *
     * @deprecated ignored by the platform; kept so 0.0.12 code compiles
     */
    @Deprecated
    public CreateDispatchJobDto withExpiresAt(Instant expiresAt) {
        CreateDispatchJobDto c = copy();
        c.expiresAt = expiresAt;
        return c;
    }

    /**
     * Not sent: the batch endpoint has no {@code idempotencyKey}; use {@link #withId}.
     *
     * @deprecated ignored by the platform; kept so 0.0.12 code compiles
     */
    @Deprecated
    public CreateDispatchJobDto withIdempotencyKey(String idempotencyKey) {
        CreateDispatchJobDto c = copy();
        c.idempotencyKey = idempotencyKey;
        return c;
    }

    /** Your own identifier for the job, 1 to 255 characters. */
    public CreateDispatchJobDto withExternalId(String externalId) {
        CreateDispatchJobDto c = copy();
        c.externalId = externalId;
        return c;
    }

    /**
     * Not sent: the batch endpoint has no {@code connectionId}.
     *
     * @deprecated ignored by the platform; kept so 0.0.12 code compiles
     */
    @Deprecated
    public CreateDispatchJobDto withConnectionId(String connectionId) {
        CreateDispatchJobDto c = copy();
        c.connectionId = connectionId;
        return c;
    }

    /**
     * The job's own dispatch priority: {@code DEFAULT} or {@code HIGH_PRIORITY},
     * matched ignoring case (written upper-case). Unset stays absent — never silently
     * defaulted — so "not asked for" stays distinguishable from an explicit {@code DEFAULT}
     * (docs/spec/dispatch-job-priority.md R3). Wins over the target
     * subscription's own priority at publish time when set.
     */
    public CreateDispatchJobDto withQueue(String queue) {
        CreateDispatchJobDto c = copy();
        c.queue = queue;
        return c;
    }

    /** What the job is, in words, for the platform's dispatch-jobs grid (1 to 255 characters). */
    public CreateDispatchJobDto withDescriptor(String descriptor) {
        CreateDispatchJobDto c = copy();
        c.descriptor = descriptor;
        return c;
    }

    /** The job id given with {@link #withId}, or null. */
    public String id() {
        return id;
    }

    public String messageGroup() {
        return messageGroup;
    }

    /**
     * Checks the job against the batch endpoint's rules (framework write-path contract 7a).
     *
     * @throws IllegalArgumentException naming the first rule broken
     */
    public void validate() {
        if (code == null || !CODE.matcher(code).matches() || code.codePointCount(0, code.length()) > 255) {
            throw new IllegalArgumentException("dispatch job code must be {application}:{rest} "
                    + "(application [a-z][a-z0-9-]*), at most 255 characters: " + code);
        }
        if (!validTargetUrl(targetUrl)) {
            throw new IllegalArgumentException(
                    "dispatch job targetUrl must be an absolute http(s) URL with a host and no user info");
        }
        if (id != null && !PLATFORM_ID.matcher(id).matches()) {
            throw new IllegalArgumentException("id must match [0-9A-Za-z_-]{1,17}");
        }
        Map<String, String> ids = new LinkedHashMap<>();
        ids.put("eventId", eventId);
        ids.put("platformClientId", platformClientId);
        ids.put("subscriptionId", subscriptionId);
        ids.put("serviceAccountId", serviceAccountId);
        ids.put("dispatchPoolId", dispatchPoolId);
        for (Map.Entry<String, String> e : ids.entrySet()) {
            if (e.getValue() != null && !PLATFORM_ID.matcher(e.getValue()).matches()) {
                throw new IllegalArgumentException(e.getKey() + " must match [0-9A-Za-z_-]{1,17}");
            }
        }
        if (platformClientId != null && clientCode != null) {
            throw new IllegalArgumentException("set platformClientId or clientCode, not both "
                    + "(the platform ignores the code when an id is given)");
        }
        if (clientCode != null && (clientCode.isBlank() || length(clientCode.strip()) > 100)) {
            throw new IllegalArgumentException("clientCode must be 1 to 100 characters");
        }
        atLeastOne("sequence", sequence);
        atLeastOne("timeoutSeconds", timeoutSeconds);
        atLeastOne("maxRetries", maxRetries);
        if (source != null && (source.isBlank() || length(source) > 255)) {
            throw new IllegalArgumentException("source must be 1 to 255 characters");
        }
        Map<String, String> texts = new LinkedHashMap<>();
        texts.put("messageGroup", messageGroup);
        texts.put("externalId", externalId);
        texts.put("subject", subject);
        texts.put("descriptor", descriptor);
        for (Map.Entry<String, String> e : texts.entrySet()) {
            if (e.getValue() != null && (e.getValue().isBlank() || length(e.getValue()) > 255)) {
                throw new IllegalArgumentException(e.getKey() + " must be 1 to 255 characters");
            }
        }
        if (queue != null) {
            String q = queue.strip().toUpperCase(Locale.ROOT);
            if (!q.equals("DEFAULT") && !q.equals("HIGH_PRIORITY")) {
                throw new IllegalArgumentException("queue must be DEFAULT or HIGH_PRIORITY");
            }
        }
        if (metadata.size() > 100) {
            throw new IllegalArgumentException("metadata must have at most 100 entries");
        }
        for (String key : metadata.keySet()) {
            if (key == null || key.isBlank()) {
                throw new IllegalArgumentException("metadata keys must not be blank");
            }
        }
        if (payloadContentType == null || payloadContentType.isEmpty() || length(payloadContentType) > 100) {
            throw new IllegalArgumentException("payloadContentType must be 1 to 100 characters");
        }
    }

    /**
     * The batch item for a direct {@code POST /api/dispatch-jobs/batch}: validated, defaults
     * applied, {@code id} only when one was given (the platform mints one otherwise).
     *
     * @throws IllegalArgumentException when {@link #validate()} refuses the job
     */
    public Map<String, Object> toBatchItem() {
        validate();
        return payload(id);
    }

    /** The outbox payload: validated, with {@code rowId} as the job id. */
    Map<String, Object> toPayload(String rowId) {
        validate();
        return payload(rowId);
    }

    /** The batch item, members in the framework's order (conformance vector). */
    private Map<String, Object> payload(String jobId) {
        Map<String, Object> p = new LinkedHashMap<>();
        putIfNotNull(p, "id", jobId);
        putIfNotNull(p, "kind", kind != null ? kind.name() : null);
        p.put("code", code);
        p.put("source", source != null ? source : code.substring(0, code.indexOf(':')));
        putIfNotNull(p, "subject", subject);
        p.put("targetUrl", targetUrl);
        p.put("payload", payload != null ? payload : "{}");
        p.put("payloadContentType", payloadContentType);
        p.put("dataOnly", dataOnly);
        putIfNotNull(p, "externalId", externalId);
        putIfNotNull(p, "eventId", eventId);
        putIfNotNull(p, "correlationId", correlationId);
        putIfNotNull(p, "clientId", platformClientId);
        putIfNotNull(p, "clientCode", clientCode != null ? clientCode.strip() : null);
        putIfNotNull(p, "subscriptionId", subscriptionId);
        putIfNotNull(p, "serviceAccountId", serviceAccountId);
        putIfNotNull(p, "dispatchPoolId", dispatchPoolId);
        putIfNotNull(p, "messageGroup", messageGroup);
        putIfNotNull(p, "mode", mode != null ? mode.name() : null);
        putIfNotNull(p, "sequence", sequence);
        putIfNotNull(p, "timeoutSeconds", timeoutSeconds);
        putIfNotNull(p, "maxRetries", maxRetries);
        if (!metadata.isEmpty()) {
            List<Map<String, String>> pairs = new ArrayList<>(metadata.size());
            metadata.forEach((k, v) -> {
                Map<String, String> pair = new LinkedHashMap<>();
                pair.put("key", k);
                pair.put("value", v);
                pairs.add(pair);
            });
            p.put("metadata", pairs);
        }
        putIfNotNull(p, "queue", queue != null ? queue.strip().toUpperCase(Locale.ROOT) : null);
        putIfNotNull(p, "descriptor", descriptor);
        return p;
    }

    private static void atLeastOne(String name, Integer value) {
        if (value != null && value < 1) {
            throw new IllegalArgumentException(
                    name + " must be at least 1 (the platform turns 0 into its default)");
        }
    }

    private static int length(String s) {
        return s.codePointCount(0, s.length());
    }

    private static boolean validTargetUrl(String raw) {
        if (raw == null) {
            return false;
        }
        try {
            URI uri = new URI(raw);
            String scheme = uri.getScheme();
            // getHost() is null for a registry-based authority (a host name with an underscore,
            // which the platform accepts); the authority then has to be a bare host[:port].
            String authority = uri.getRawAuthority();
            boolean hasHost = (uri.getHost() != null && !uri.getHost().isEmpty())
                    || (authority != null && !authority.isEmpty() && !authority.contains("@")
                            && !authority.startsWith(":"));
            return uri.isAbsolute()
                    && ("http".equalsIgnoreCase(scheme) || "https".equalsIgnoreCase(scheme))
                    && hasHost
                    && uri.getRawUserInfo() == null
                    && (authority == null || !authority.contains("@"));
        } catch (URISyntaxException e) {
            return false;
        }
    }

    private static void putIfNotNull(Map<String, Object> map, String key, Object value) {
        if (value != null) map.put(key, value);
    }
}
