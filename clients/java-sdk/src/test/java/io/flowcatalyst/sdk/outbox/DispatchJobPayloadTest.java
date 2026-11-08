package io.flowcatalyst.sdk.outbox;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/**
 * DISPATCH_JOB outbox rows are POST /api/dispatch-jobs/batch items the platform accepts
 * (framework write-path contract 7a, platform docs/platform-bug-fixes.md F14): metadata as the
 * key/value array (an object fails the whole batch), a client by its identifier
 * ({@code clientCode}), the job id in the payload and as the row id, and none of the members the
 * batch endpoint ignores.
 */
class DispatchJobPayloadTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private static JsonNode written(CreateDispatchJobDto job) {
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        String id = new OutboxManager(driver, "clt_TEST123456789").createDispatchJob(job);
        OutboxMessage row = driver.inserted.getFirst();
        assertEquals(id, row.id());
        JsonNode payload = MAPPER.readTree(row.payload());
        assertEquals(id, payload.get("id").asString(), "the payload id is the row id");
        return payload;
    }

    @Test
    void metadataIsTheKeyValueArrayInInsertionOrder() {
        Map<String, String> more = new LinkedHashMap<>();
        more.put("b", "2");
        more.put("a", "1");
        JsonNode payload = written(CreateDispatchJobDto
                .create("svc", "app:sub:agg:act", "https://example.com/hook", "{}", "pool-1")
                .withMetadata("z", "0")
                .withMetadata(more)
                .withMetadata(Map.of("z", "9")));

        JsonNode metadata = payload.get("metadata");
        assertTrue(metadata.isArray(), "metadata must be an array: " + metadata);
        assertEquals("[{\"key\":\"z\",\"value\":\"9\"},{\"key\":\"b\",\"value\":\"2\"},"
                + "{\"key\":\"a\",\"value\":\"1\"}]", metadata.toString());
    }

    @Test
    void membersTheBatchEndpointIgnoresAreNotWritten() {
        @SuppressWarnings("deprecation")
        CreateDispatchJobDto job = CreateDispatchJobDto
                .create("svc", "app:sub:agg:act", "https://example.com/hook", "{}", "pool-1")
                .withHeaders(Map.of("X-A", "1"))
                .withRetryStrategy("EXPONENTIAL")
                .withScheduledFor(Instant.parse("2026-10-13T00:00:00Z"))
                .withExpiresAt(Instant.parse("2026-10-14T00:00:00Z"));
        JsonNode payload = written(job);
        for (String ignored : List.of("headers", "retryStrategy", "scheduledFor", "expiresAt")) {
            assertFalse(payload.has(ignored), ignored + " is ignored by the batch endpoint");
        }
    }

    @Test
    void connectionIdAndIdempotencyKeyAreWrittenWhenSetAndOmittedWhenNot() {
        JsonNode payload = written(CreateDispatchJobDto.of("orders:export", "https://e.com/x")
                .withConnectionId("con_1").withIdempotencyKey("  order-42  "));
        assertEquals("con_1", payload.get("connectionId").asString());
        assertEquals("  order-42  ", payload.get("idempotencyKey").asString(), "the key is written untrimmed");

        JsonNode unset = written(CreateDispatchJobDto.of("orders:export", "https://e.com/x"));
        assertFalse(unset.has("connectionId"));
        assertFalse(unset.has("idempotencyKey"));
    }

    @Test
    void anOverlongIdempotencyKeyOrBadConnectionIdIsRefused() {
        assertThrows(IllegalArgumentException.class, () -> CreateDispatchJobDto
                .of("a:b", "https://e.com").withIdempotencyKey("k".repeat(101)).validate());
        assertThrows(IllegalArgumentException.class, () -> CreateDispatchJobDto
                .of("a:b", "https://e.com").withConnectionId("not a valid id!").validate());
        CreateDispatchJobDto.of("a:b", "https://e.com").withIdempotencyKey("k".repeat(100)).validate();
    }

    @Test
    void createKeepsItsEarlierDefaultsAndFillsWhatThePlatformCannotDefault() {
        JsonNode payload = written(CreateDispatchJobDto
                .create(null, "app:sub:agg:act", "https://example.com/hook", null, null));
        assertTrue(payload.get("dataOnly").asBoolean(), "create(...) keeps dataOnly true");
        assertEquals(30, payload.get("timeoutSeconds").asInt());
        assertEquals(5, payload.get("maxRetries").asInt());
        assertFalse(payload.has("kind"), "create(...) leaves the kind to the platform (EVENT)");
        assertEquals("app", payload.get("source").asString(), "source defaults to the application segment");
        assertEquals("{}", payload.get("payload").asString(), "payload is NOT NULL on the platform");
        assertEquals("application/json", payload.get("payloadContentType").asString());
        assertFalse(payload.has("dispatchPoolId"));
    }

    @Test
    void ofUsesThePlatformsDefaults() {
        JsonNode payload = written(CreateDispatchJobDto.of("orders:export", "https://example.com/x"));
        assertEquals("TASK", payload.get("kind").asString());
        assertEquals("orders", payload.get("source").asString());
        assertFalse(payload.get("dataOnly").asBoolean());
        assertEquals("{}", payload.get("payload").asString());
        for (String unset : List.of("timeoutSeconds", "maxRetries", "sequence", "mode", "queue",
                "metadata", "clientId", "clientCode")) {
            assertFalse(payload.has(unset), unset + " is left to the platform");
        }
    }

    @Test
    void aGivenIdIsTheRowIdAndThePayloadId() {
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        String id = new OutboxManager(driver, "clt_1")
                .createDispatchJob(CreateDispatchJobDto.of("orders:export", "https://e.com/x").withId("job_1"));
        assertEquals("job_1", id);
        assertEquals("job_1", driver.inserted.getFirst().id());
    }

    @Test
    void aClientByItsIdentifierIsClientCode() {
        JsonNode payload = written(CreateDispatchJobDto.of("orders:export", "https://e.com/x")
                .withClientCode("  acme  "));
        assertEquals("acme", payload.get("clientCode").asString());
        assertFalse(payload.has("clientId"));

        JsonNode byId = written(CreateDispatchJobDto.of("orders:export", "https://e.com/x")
                .withPlatformClientId("0CLIENT0000001"));
        assertEquals("0CLIENT0000001", byId.get("clientId").asString());
        assertFalse(byId.has("clientCode"));
    }

    @Test
    void clientIdAndClientCodeTogetherAreRefused() {
        CreateDispatchJobDto job = CreateDispatchJobDto.of("orders:export", "https://e.com/x")
                .withPlatformClientId("0CLIENT0000001").withClientCode("acme");
        IllegalArgumentException e = assertThrows(IllegalArgumentException.class, job::validate);
        assertEquals("set platformClientId or clientCode, not both (the platform ignores the code "
                + "when an id is given)", e.getMessage());
        assertThrows(IllegalArgumentException.class,
                () -> CreateDispatchJobDto.of("orders:export", "https://e.com/x").withClientCode(" ").validate());
    }

    @Test
    void zeroForAPlatformDefaultedNumberIsRefused() {
        for (CreateDispatchJobDto job : List.of(
                CreateDispatchJobDto.of("a:b", "https://e.com").withTimeoutSeconds(0),
                CreateDispatchJobDto.of("a:b", "https://e.com").withMaxRetries(0),
                CreateDispatchJobDto.of("a:b", "https://e.com").withSequence(0))) {
            assertThrows(IllegalArgumentException.class, job::validate);
        }
    }

    @Test
    void theQueueIsCheckedAndUpperCased() {
        assertEquals("HIGH_PRIORITY", written(CreateDispatchJobDto.of("a:b", "https://e.com")
                .withQueue("high_priority")).get("queue").asString());
        assertThrows(IllegalArgumentException.class,
                () -> CreateDispatchJobDto.of("a:b", "https://e.com").withQueue("URGENT").validate());
    }

    @Test
    void toBatchItemIsThePayloadWithItsOwnIdOrNone() {
        Map<String, Object> item = CreateDispatchJobDto.of("a:b", "https://e.com").toBatchItem();
        assertFalse(item.containsKey("id"), "without an id the platform mints one");
        assertEquals("TASK", item.get("kind"));
        assertEquals("j1", CreateDispatchJobDto.of("a:b", "https://e.com").withId("j1").toBatchItem().get("id"));
        assertThrows(IllegalArgumentException.class,
                () -> CreateDispatchJobDto.of("A:b", "https://e.com").toBatchItem());
    }

    // ── the client a job defaults to, and the idempotency key ────────────

    private static final String KEY_ERROR = "idempotencyKey must not be blank and at most 100 bytes (UTF-8)";

    private static CreateDispatchJobDto job() {
        return CreateDispatchJobDto.of("shop:x", "https://example.com/hook").withId("j");
    }

    private static JsonNode writtenBy(String clientCode, CreateDispatchJobDto job) {
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        new OutboxManager(driver, "clt_TEST123456789", clientCode, java.time.InstantSource.system())
                .createDispatchJob(job);
        return MAPPER.readTree(driver.inserted.getFirst().payload());
    }

    @Test
    void aJobNamingNoClientGetsTheWritersClientCode() {
        assertEquals("acme", writtenBy("acme", job()).get("clientCode").asString());
    }

    @Test
    void thePlatformClientGetsNoClientCode() {
        assertFalse(writtenBy(OutboxManager.PLATFORM_CLIENT_CODE, job()).has("clientCode"));
        assertFalse(writtenBy(null, job()).has("clientCode"), "no configured code, none written");
    }

    @Test
    void theJobsOwnClientWins() {
        assertEquals("other", writtenBy("acme", job().withClientCode("other")).get("clientCode").asString());
        assertEquals("other",
                writtenBy(OutboxManager.PLATFORM_CLIENT_CODE, job().withClientCode("other")).get("clientCode").asString());
        JsonNode byId = writtenBy("acme", job().withPlatformClientId("clt_1"));
        assertFalse(byId.has("clientCode"), "platformClientId suppresses the default");
        assertEquals("clt_1", byId.get("clientId").asString());
    }

    @Test
    void anIdempotencyKeyIsWrittenExactlyAsGiven() {
        assertEquals("  order-42  ", writtenBy(null, job().withIdempotencyKey("  order-42  "))
                .get("idempotencyKey").asString());
        assertFalse(writtenBy(null, job()).has("idempotencyKey"));
    }

    @Test
    void anIdempotencyKeyIsLimitedToHundredBytesNotCharacters() {
        job().withIdempotencyKey("k".repeat(100)).validate();
        job().withIdempotencyKey("é".repeat(50)).validate();
        for (String bad : List.of("   ", "k".repeat(101), "é".repeat(51))) {
            IllegalArgumentException e = assertThrows(IllegalArgumentException.class,
                    () -> job().withIdempotencyKey(bad).validate());
            assertEquals(KEY_ERROR, e.getMessage());
        }
    }

    @Test
    void connectionIdAndIdempotencyKeyComeLast() {
        JsonNode p = writtenBy(null, job().withIdempotencyKey("k").withConnectionId("con_1")
                .withDescriptor("d").withQueue("DEFAULT"));
        List<String> names = new java.util.ArrayList<>();
        p.propertyNames().forEach(names::add);
        assertEquals(List.of("queue", "descriptor", "connectionId", "idempotencyKey"),
                names.subList(names.size() - 4, names.size()));
    }
}
