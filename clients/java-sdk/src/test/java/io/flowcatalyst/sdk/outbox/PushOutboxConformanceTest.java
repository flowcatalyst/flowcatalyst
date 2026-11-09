package io.flowcatalyst.sdk.outbox;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

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
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

/**
 * The PUSH outbox vector of the application framework
 * ({@code flowcatalyst-framework/conformance/outbox/outbox-push.json}; docs/push-gateway-spec.md
 * section 6.1), copied into {@code src/test/resources/conformance}. The SDK's writer is held to it:
 * every column of a PUSH row, the payload text byte for byte, and the builder's refusal codes.
 *
 * <p>When the framework checkout is beside this repository (or {@code FC_FRAMEWORK_DIR} names it),
 * the copy is checked against it, and against the Go SDK's pinned copy while it sits in this
 * repository, so a vector change there fails here until copied.
 */
class PushOutboxConformanceTest {

    private static final ObjectMapper JSON = JsonMapper.builder().build();
    private static final DateTimeFormatter MICROS =
            DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss.SSSSSS'Z'").withZone(ZoneOffset.UTC);

    private static final Path VECTOR = Path.of("src", "test", "resources", "conformance", "outbox-push.json");

    private static JsonNode vector() throws IOException {
        return JSON.readTree(Files.readString(VECTOR));
    }

    @Test
    void theVectorIsTheFrameworksAndTheGoSdksCopy() throws IOException {
        Path framework = frameworkRoot();
        Assumptions.assumeTrue(framework != null,
                "framework checkout not found (set FC_FRAMEWORK_DIR to compare the copy)");
        assertArrayEquals(
                Files.readAllBytes(framework.resolve("conformance").resolve("outbox").resolve("outbox-push.json")),
                Files.readAllBytes(VECTOR),
                "src/test/resources/conformance/outbox-push.json differs from the framework's: copy it again");
        Path go = Path.of("..", "..", "pkg", "fcsdk", "outbox", "testdata", "conformance", "outbox-push.json");
        if (Files.exists(go)) {
            assertArrayEquals(Files.readAllBytes(go), Files.readAllBytes(VECTOR),
                    "differs from the Go SDK's pinned copy");
        }
    }

    static Path frameworkRoot() {
        String env = System.getenv("FC_FRAMEWORK_DIR");
        List<Path> candidates = env != null && !env.isBlank()
                ? List.of(Path.of(env))
                : List.of(Path.of("..", "..", "..", "flowcatalyst-framework"),
                        Path.of("..", "..", "flowcatalyst-framework"));
        for (Path root : candidates) {
            if (Files.isDirectory(root.resolve("conformance").resolve("outbox"))) {
                return root;
            }
        }
        return null;
    }

    @Test
    void thePushRowsAreTheVectorsRows() throws IOException {
        JsonNode cases = vector().get("cases");
        assertTrue(cases.size() > 0, "no cases");
        for (JsonNode c : cases) {
            String name = c.get("name").asString();
            Instant now = Instant.parse(c.get("now").asString());
            String client = c.get("context").get("client").asString();

            OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
            OutboxManager outbox = new OutboxManager(driver, client, client, InstantSource.fixed(now));
            for (JsonNode p : c.get("pushes")) {
                outbox.createPush(build(p));
            }

            List<JsonNode> want = new ArrayList<>();
            for (JsonNode r : c.get("rows")) {
                if (r.get("type").asString().equals("PUSH")) {
                    want.add(r);
                }
            }
            assertEquals(want.size(), driver.inserted.size(), name + ": row count");
            for (int i = 0; i < want.size(); i++) {
                String at = name + " push " + i;
                JsonNode w = want.get(i);
                OutboxMessage got = driver.inserted.get(i);
                assertEquals(w.get("id").asString(), got.id(), at + ": id");
                assertEquals(OutboxMessage.MessageType.PUSH, got.type(), at + ": type");
                assertEquals(text(w, "message_group"), got.messageGroup(), at + ": message_group");
                assertEquals(w.get("status").asInt(), got.status(), at + ": status");
                assertEquals(text(w, "client_id"), got.clientId(), at + ": client_id");
                assertTrue(w.get("headers").isNull(), at + ": vector headers");
                assertNull(got.headers(), at + ": headers");
                assertEquals(w.get("created_at").asString(), MICROS.format(got.createdAt()), at + ": created_at");
                assertEquals(got.createdAt(), got.updatedAt(), at + ": updated_at");
                assertEquals(w.get("payloadText").asString(), got.payload(), at + ": payload bytes, members in order");
                assertEquals(w.get("payload").toString(), JSON.readTree(got.payload()).toString(), at + ": payload");
                assertEquals(w.get("payload_size").asInt(), got.payloadSize(), at + ": payload_size");
            }
        }
    }

    @Test
    void theBuilderRefusesTheVectorsInvalidPushes() throws IOException {
        JsonNode invalid = vector().get("invalidPushes");
        assertTrue(invalid.size() > 0, "no invalid-push vectors");
        for (JsonNode in : invalid) {
            String want = in.get("code").asString();
            CreatePushDto push = build(in);
            PushException e = assertThrows(PushException.class, push::validate, in + ": accepted");
            assertEquals(want, e.code().name(), in.toString());

            OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
            assertThrows(PushException.class, () -> new OutboxManager(driver, "cli_1").createPush(push),
                    in + ": written although invalid");
            assertTrue(driver.inserted.isEmpty(), in + ": a refused push left a row");
        }
    }

    @Test
    void everyCodeOfThePushContractIsReachable() {
        Map<PushException.Code, CreatePushDto> bad = Map.of(
                PushException.Code.TOPIC_INVALID, CreatePushDto.create("nope", "a", Map.of()),
                PushException.Code.TYPE_INVALID, CreatePushDto.create("a:b", "A", Map.of()),
                PushException.Code.TYPE_RESERVED, CreatePushDto.create("a:b", "fw.x", Map.of()),
                PushException.Code.DATA_INVALID, CreatePushDto.create("a:b", "c", "{"),
                PushException.Code.DATA_NOT_OBJECT, CreatePushDto.create("a:b", "c", List.of(1)),
                PushException.Code.DATA_TOO_LARGE,
                        CreatePushDto.create("a:b", "c", Map.of("x", "y".repeat(100))).withMaxDataBytes(50),
                PushException.Code.PUSH_ID_INVALID, CreatePushDto.create("a:b", "c", Map.of()).withId("a b"),
                PushException.Code.CLIENT_INVALID, CreatePushDto.create("a:b", "c", Map.of()).withClientCode("  "));
        for (PushException.Code code : PushException.Code.values()) {
            assertEquals(code, assertThrows(PushException.class, bad.get(code)::validate).code());
        }
    }

    @Test
    void dataIsAnObjectJsonTextOrAJsonNodeAndCompactedWithoutRewritingValues() {
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        OutboxManager outbox = new OutboxManager(driver, "cli_1", "acme", InstantSource.system());
        outbox.createPush(CreatePushDto.create("shop:a", "t", " { \"n\" : 1.50 , \"s\" : \"a b\" } ").withId("p1"));
        outbox.createPush(CreatePushDto.create("shop:a", "t", JSON.createObjectNode().put("k", 1)).withId("p2"));
        assertEquals("{\"topic\":\"shop:a\",\"type\":\"t\",\"data\":{\"n\":1.50,\"s\":\"a b\"},"
                + "\"clientCode\":\"acme\",\"pushId\":\"p1\"}", driver.inserted.get(0).payload());
        assertEquals("{\"k\":1}", JSON.readTree(driver.inserted.get(1).payload()).get("data").toString());
        assertEquals("p1", driver.inserted.get(0).id(), "the row id is the push id");
        // text after the value is not JSON
        assertThrows(PushException.class, () -> CreatePushDto.create("shop:a", "t", "{} x").validate());
    }

    @Test
    void anIdlessPushGetsATsidRowIdAndPushId() {
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        String id = new OutboxManager(driver, "cli_1").createPush(CreatePushDto.create("shop:a", "t", Map.of()));
        assertTrue(Tsid.isValid(id) && id.length() == 13);
        assertEquals(id, driver.inserted.getFirst().id());
        assertTrue(driver.inserted.getFirst().payload().endsWith("\"pushId\":\"" + id + "\"}"));
    }

    @Test
    void thePushInsertStatementIsTheGoSdksStatement() {
        // pkg/fcsdk/outbox/push.go PushInsertStatement, with ? for $n
        String want = "WITH woken AS (SELECT pg_notify('fc_outbox_push', ''))\n"
                + "INSERT INTO outbox_messages (\n"
                + "  id, type, message_group, payload, status, created_at, updated_at,\n"
                + "  client_id, payload_size, headers\n"
                + ")\n"
                + "SELECT ?::text, ?::text, ?::text, ?::text, ?::smallint, ?::timestamptz, ?::timestamptz,\n"
                + "  ?::text, ?::integer, ?::jsonb\n"
                + "FROM woken";
        assertEquals(want, new JdbcOutboxDriver(null).pushInsertSql());
        assertEquals(want.replace("outbox_messages", "my_outbox"), new JdbcOutboxDriver(null, "my_outbox").pushInsertSql());
        assertTrue(StandardCharsets.UTF_8.newEncoder().canEncode(want));
    }

    /** A push from a vector input, through {@link CreatePushDto}. */
    private static CreatePushDto build(JsonNode in) {
        Object data;
        if (in.has("dataText")) {
            data = in.get("dataText").asString();
        } else if (in.has("padBytes")) {
            data = Map.of("blob", "x".repeat(in.get("padBytes").asInt()));
        } else {
            data = in.get("data");
        }
        CreatePushDto p = CreatePushDto.create(in.get("topic").asString(), in.get("type").asString(), data);
        if (in.hasNonNull("id")) p = p.withId(in.get("id").asString());
        if (in.hasNonNull("clientCode")) p = p.withClientCode(in.get("clientCode").asString());
        if (in.has("ungrouped") && in.get("ungrouped").asBoolean()) p = p.withUngrouped();
        if (in.hasNonNull("messageGroup")) p = p.withMessageGroup(in.get("messageGroup").asString());
        return p;
    }

    private static String text(JsonNode node, String field) {
        JsonNode v = node.get(field);
        return v == null || v.isNull() ? null : v.asString();
    }
}
