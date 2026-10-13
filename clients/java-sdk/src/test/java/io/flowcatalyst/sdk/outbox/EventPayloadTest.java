package io.flowcatalyst.sdk.outbox;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.time.Instant;
import java.time.InstantSource;
import java.util.Map;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/**
 * EVENT and AUDIT_LOG rows against the platform's ingest (docs/platform-bug-fixes.md F13, F14):
 * a missing source is left out so the platform defaults it, every event carries a
 * deduplication id so a batch the outbox processor sends again does not create a second event,
 * and an injected clock dates the row and the audit entry.
 */
class EventPayloadTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void aBlankSourceIsLeftOutForThePlatformToDefault() {
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        OutboxManager outbox = new OutboxManager(driver, "clt_1");
        outbox.createEvent(CreateEventDto.create("orders:sales:order:created", Map.of()).withSource("  "));
        outbox.createEvent(CreateEventDto.create("orders:sales:order:created", Map.of()));
        for (OutboxMessage row : driver.inserted) {
            assertFalse(MAPPER.readTree(row.payload()).has("source"), row.payload());
        }
    }

    @Test
    void theDeduplicationIdDefaultsToTheRowId() {
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        OutboxManager outbox = new OutboxManager(driver, "clt_1");
        String id = outbox.createEvent(CreateEventDto.create("orders:sales:order:created", Map.of()));
        assertEquals(id, MAPPER.readTree(driver.inserted.getFirst().payload()).get("deduplicationId").asString());

        outbox.createEvent(CreateEventDto.create("orders:sales:order:created", Map.of())
                .withDeduplicationId("mine"));
        assertEquals("mine", MAPPER.readTree(driver.inserted.get(1).payload()).get("deduplicationId").asString());
    }

    @Test
    void dataStaysOneEncodedJsonDocument() {
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        new OutboxManager(driver, "clt_1").createEvent(
                CreateEventDto.create("orders:sales:order:created", Map.of("orderId", "o1")));
        JsonNode data = MAPPER.readTree(driver.inserted.getFirst().payload()).get("data");
        // Encoded once (the platform unwraps it at ingest, F13): a string holding the object,
        // never a string holding a string.
        assertTrue(data.isString());
        assertTrue(MAPPER.readTree(data.asString()).isObject());
    }

    @Test
    void anInjectedClockDatesTheRowAndTheAuditEntry() {
        Instant now = Instant.parse("2026-10-13T09:00:00.250Z");
        OutboxManagerTest.CapturingDriver driver = new OutboxManagerTest.CapturingDriver();
        OutboxManager outbox = new OutboxManager(driver, "clt_1", InstantSource.fixed(now));
        outbox.createAuditLog(CreateAuditLogDto.create("Order", "o1", "place"));
        OutboxMessage row = driver.inserted.getFirst();
        assertEquals(now, row.createdAt());
        assertEquals(now, row.updatedAt());
        assertEquals("2026-10-13T09:00:00.250Z",
                MAPPER.readTree(row.payload()).get("performedAt").asString());
    }
}
