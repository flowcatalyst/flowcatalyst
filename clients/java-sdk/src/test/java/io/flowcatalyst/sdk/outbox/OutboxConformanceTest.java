package io.flowcatalyst.sdk.outbox;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.fail;

import io.flowcatalyst.sdk.tsid.Tsid;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;
import java.time.InstantSource;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;
import tools.jackson.databind.node.ObjectNode;

/**
 * The outbox conformance vectors of the application framework
 * ({@code flowcatalyst-framework/conformance/outbox/outbox.json} and
 * {@code conformance/sql/outbox/insert.sql}), copied into
 * {@code src/test/resources/conformance}. The SDK's writers are held to them: every column the
 * SDK writes, the payload text byte for byte (the framework's writer is the reference, and this
 * SDK writes the same encoded {@code data} / {@code operationData} strings), and the
 * dispatch-job builder's refusals with the vector's messages.
 *
 * <p>One allowed difference: an event row's {@code id}. The framework stores the event id as the
 * row id; the SDK's row id is its own generated TSID, and the event id travels as
 * {@code deduplicationId} (exactly as the vector's payload carries it). Every dispatch-job row
 * id is the job id, as in the vector.
 *
 * <p>When the framework checkout is beside this repository (or {@code FC_FRAMEWORK_DIR} names
 * it), the copies are checked against it, so a vector change there fails here until copied.
 */
class OutboxConformanceTest {

    private static final ObjectMapper JSON = JsonMapper.builder().build();
    private static final DateTimeFormatter MICROS =
            DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss.SSSSSS'Z'").withZone(ZoneOffset.UTC);

    private static Path vector(String name) {
        return Path.of("src", "test", "resources", "conformance", name);
    }

    private static JsonNode vectors() throws IOException {
        return JSON.readTree(Files.readString(vector("outbox.json")));
    }

    // ── the copies are the framework's files ───────────────────────────

    private static Path frameworkConformanceDir() {
        String env = System.getenv("FC_FRAMEWORK_DIR");
        List<Path> candidates = env != null && !env.isBlank()
                ? List.of(Path.of(env))
                // flowcatalyst-go/clients/java-sdk, or flowcatalyst-javalin/sdk (the mirror)
                : List.of(Path.of("..", "..", "..", "flowcatalyst-framework"),
                        Path.of("..", "..", "flowcatalyst-framework"));
        for (Path root : candidates) {
            Path dir = root.resolve("conformance");
            if (Files.isDirectory(dir.resolve("outbox"))) {
                return dir;
            }
        }
        return null;
    }

    @Test
    void theVectorsAreTheFrameworksCopy() throws IOException {
        Path src = frameworkConformanceDir();
        Assumptions.assumeTrue(src != null,
                "framework checkout not found (set FC_FRAMEWORK_DIR to compare the copy)");
        assertArrayEquals(Files.readAllBytes(src.resolve("outbox").resolve("outbox.json")),
                Files.readAllBytes(vector("outbox.json")),
                "src/test/resources/conformance/outbox.json differs from the framework's: copy it again");
        assertArrayEquals(Files.readAllBytes(src.resolve("sql").resolve("outbox").resolve("insert.sql")),
                Files.readAllBytes(vector("insert.sql")),
                "src/test/resources/conformance/insert.sql differs from the framework's: copy it again");
    }

    @Test
    void theInsertStatementIsTheFrameworksStatement() throws IOException {
        String want = Files.readString(vector("insert.sql"), StandardCharsets.UTF_8);
        assertEquals(want, new JdbcOutboxDriver(null).insertSql());
        assertEquals(want.replace("outbox_messages", "my_outbox"),
                new JdbcOutboxDriver(null, "my_outbox").insertSql());
    }

    // ── rows ──────────────────────────────────────────────────────────

    @Test
    void theRowsAreTheVectorsRows() throws IOException {
        JsonNode cases = vectors().get("cases");
        assertTrue(cases.size() > 0, "no cases");
        for (JsonNode c : cases) {
            String name = c.get("name").asString();
            Instant now = Instant.parse(c.get("now").asString());
            JsonNode ctx = c.get("context");
            String client = text(ctx, "client");
            String correlation = text(ctx, "correlation");

            OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
            OutboxManager outbox = new OutboxManager(driver, client, InstantSource.fixed(now));

            // The framework's write order: the events, the dispatch jobs, the audit row.
            List<CreateEventDto> events = new ArrayList<>();
            for (JsonNode ev : c.get("events")) {
                events.add(event(ev, ctx));
            }
            outbox.createEvents(events);
            for (JsonNode job : c.get("dispatchJobs")) {
                outbox.createDispatchJob(dispatchJob(job).withCorrelationId(correlation));
            }
            outbox.createAuditLog(audit(c.get("audit"), ctx));

            List<OutboxMessage> rows = driver.inserted;
            JsonNode want = c.get("rows");
            assertEquals(want.size(), rows.size(), name + ": row count");
            for (int i = 0; i < want.size(); i++) {
                String at = name + " row " + i;
                JsonNode w = want.get(i);
                OutboxMessage got = rows.get(i);

                String wantId = w.get("id").asString();
                if (wantId.equals("<tsid>") || got.type() == OutboxMessage.MessageType.EVENT) {
                    // The allowed difference (class doc): an event's row id is the SDK's own.
                    assertTrue(Tsid.isValid(got.id()) && got.id().length() == 13,
                            at + ": id " + got.id() + " is not a 13-character TSID");
                } else {
                    assertEquals(wantId, got.id(), at + ": id");
                }
                assertEquals(w.get("type").asString(), got.type().name(), at + ": type");
                assertEquals(nullable(w, "message_group"), got.messageGroup(), at + ": message_group");
                assertEquals(w.get("status").asInt(), got.status(), at + ": status");
                assertEquals(0, w.get("retry_count").asInt(), at + ": retry_count is the column default");
                assertEquals(nullable(w, "client_id"), got.clientId(), at + ": client_id");
                assertTrue(w.get("headers").isNull(), at + ": vector headers");
                assertNull(got.headers(), at + ": headers");
                assertEquals(w.get("created_at").asString(), MICROS.format(got.createdAt()), at + ": created_at");
                assertEquals(got.createdAt(), got.updatedAt(), at + ": updated_at");

                assertEquals(canonical(w.get("payloadText").asString()), canonical(got.payload()),
                        at + ": payload (parsed)");
                assertEquals(w.get("payloadText").asString(), got.payload(), at + ": payload text");
                assertEquals(w.get("payload_size").asInt(), got.payloadSize(), at + ": payload_size");
            }
        }
    }

    @Test
    void theBuilderRefusesTheVectorsInvalidDispatchJobs() throws IOException {
        JsonNode invalid = vectors().get("invalidDispatchJobs");
        assertTrue(invalid.size() > 0, "no invalid dispatch-job vectors");
        for (JsonNode in : invalid) {
            String want = in.get("error").asString();
            CreateDispatchJobDto job = dispatchJob(in);
            IllegalArgumentException e = assertThrows(IllegalArgumentException.class, job::validate,
                    in + ": accepted, the vector refuses it");
            assertEquals(want, e.getMessage(), in.toString());

            OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
            assertThrows(IllegalArgumentException.class,
                    () -> new OutboxManager(driver, "acme").createDispatchJob(job),
                    in + ": written although invalid");
            assertTrue(driver.inserted.isEmpty(), in + ": a refused job left a row");
        }
    }

    // ── the vector's inputs, as an application calls the SDK ───────────

    /** A domain event as the framework builds one (write-path contract section 6). */
    private static CreateEventDto event(JsonNode ev, JsonNode ctx) {
        String type = ev.get("type").asString();
        String[] seg = type.split(":");
        String subjectId = ev.get("subjectId").asString();
        // A record's fields as Jackson serialises them: nulls left out, declaration order.
        Map<String, Object> data = new LinkedHashMap<>();
        for (Map.Entry<String, JsonNode> f : ev.get("fields").properties()) {
            if (!f.getValue().isNull()) {
                data.put(f.getKey(), JSON.convertValue(f.getValue(), Object.class));
            }
        }
        String group = ev.has("messageGroup")
                ? ev.get("messageGroup").asString()
                : seg[1] + ":" + seg[2] + ":" + subjectId;
        CreateEventDto dto = CreateEventDto.create(type, data)
                .withSource(seg[0] + ":" + seg[1])
                .withSubject(seg[1] + "." + seg[2] + "." + subjectId)
                .withCorrelationId(text(ctx, "correlation"))
                .withDeduplicationId(ev.get("eventId").asString())
                .withMessageGroup(group)
                .withClientCode(text(ctx, "client"));
        if (text(ctx, "causation") != null) {
            dto = dto.withCausationId(text(ctx, "causation"));
        }
        List<CreateEventDto.ContextEntry> contextData = new ArrayList<>();
        if (text(ctx, "principal") != null) {
            contextData.add(new CreateEventDto.ContextEntry("principalId", text(ctx, "principal")));
        }
        if (text(ctx, "execution") != null) {
            contextData.add(new CreateEventDto.ContextEntry("executionId", text(ctx, "execution")));
        }
        return contextData.isEmpty() ? dto : dto.withContextData(contextData);
    }

    @SuppressWarnings("unchecked")
    private static CreateAuditLogDto audit(JsonNode a, JsonNode ctx) {
        CreateAuditLogDto dto = CreateAuditLogDto
                .create(a.get("entityType").asString(), a.get("entityId").asString(),
                        a.get("operation").asString())
                .withSource(a.get("source").asString())
                .withCorrelationId(text(ctx, "correlation"))
                .withClientCode(text(ctx, "client"));
        if (a.hasNonNull("data")) {
            dto = dto.withOperationData(JSON.convertValue(a.get("data"), LinkedHashMap.class));
        }
        if (text(ctx, "principal") != null) {
            dto = dto.withPrincipalId(text(ctx, "principal"));
        }
        if (text(ctx, "application") != null) {
            dto = dto.withApplicationCode(text(ctx, "application"));
        }
        return dto;
    }

    /** A dispatch job from a vector input, through {@link CreateDispatchJobDto#of}. */
    private static CreateDispatchJobDto dispatchJob(JsonNode in) {
        CreateDispatchJobDto job = CreateDispatchJobDto.of(
                in.get("code").asString(), in.get("targetUrl").asString());
        for (Map.Entry<String, JsonNode> m : in.properties()) {
            JsonNode v = m.getValue();
            job = switch (m.getKey()) {
                case "id" -> job.withId(v.asString());
                case "kind" -> job.withKind(CreateDispatchJobDto.Kind.valueOf(v.asString()));
                case "source" -> job.withSource(v.asString());
                case "subject" -> job.withSubject(v.asString());
                case "externalId" -> job.withExternalId(v.asString());
                case "eventId" -> job.withEventId(v.asString());
                case "platformClientId" -> job.withPlatformClientId(v.asString());
                case "subscriptionId" -> job.withSubscriptionId(v.asString());
                case "serviceAccountId" -> job.withServiceAccountId(v.asString());
                case "dispatchPoolId" -> job.withDispatchPoolId(v.asString());
                case "messageGroup" -> job.withMessageGroup(v.asString());
                case "mode" -> job.withMode(CreateDispatchJobDto.DispatchMode.valueOf(v.asString()));
                case "queue" -> job.withQueue(v.asString());
                case "descriptor" -> job.withDescriptor(v.asString());
                case "dataOnly" -> job.withDataOnly(v.asBoolean());
                case "sequence" -> job.withSequence(v.asInt());
                case "timeoutSeconds" -> job.withTimeoutSeconds(v.asInt());
                case "maxRetries" -> job.withMaxRetries(v.asInt());
                case "payloadText" -> job.withPayload(v.asString(), in.get("contentType").asString());
                case "jsonPayload" -> job.withJsonPayload(JSON.convertValue(v, Object.class));
                case "metadata" -> {
                    CreateDispatchJobDto j = job;
                    for (JsonNode pair : v) {
                        j = j.withMetadata(pair.get(0).asString(), pair.get(1).asString());
                    }
                    yield j;
                }
                case "code", "targetUrl", "contentType", "error" -> job;
                default -> fail("unknown dispatch-job vector member " + m.getKey());
            };
        }
        return job;
    }

    // ── helpers ─────────────────────────────────────────────────────────

    /** The vectors' comparison rule: parsed, with a JSON document inside data / operationData parsed too. */
    private static JsonNode canonical(String payload) {
        ObjectNode node = (ObjectNode) JSON.readTree(payload);
        for (String field : List.of("data", "operationData")) {
            JsonNode v = node.get(field);
            if (v != null && v.isString()) {
                node.set(field, JSON.readTree(v.asString()));
            }
        }
        assertNotNull(node);
        assertFalse(node.isEmpty());
        return node;
    }

    private static String text(JsonNode node, String field) {
        JsonNode v = node.get(field);
        return v == null || v.isNull() ? null : v.asString();
    }

    private static String nullable(JsonNode node, String field) {
        return text(node, field);
    }
}
