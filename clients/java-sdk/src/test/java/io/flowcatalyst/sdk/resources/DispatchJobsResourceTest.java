package io.flowcatalyst.sdk.resources;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.error.FlowCatalystException;
import io.flowcatalyst.sdk.outbox.CreateDispatchJobDto;
import java.time.Duration;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/** POST /api/dispatch-jobs/batch straight from the client, with the outbox's item shape. */
class DispatchJobsResourceTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private StubServer server;

    @BeforeEach
    void setUp() throws Exception {
        server = new StubServer();
        server.stubToken("tok");
    }

    @AfterEach
    void tearDown() {
        server.close();
    }

    private FlowCatalystClient client() {
        return FlowCatalystClient.builder()
                .baseUrl(server.baseUrl())
                .clientCredentials("id", "secret")
                .retryDelay(Duration.ofMillis(1))
                .build();
    }

    @Test
    void sendsTheItemsAndReadsThePositionalResults() {
        server.on("POST", "/api/dispatch-jobs/batch", 201,
                "{\"results\":[{\"id\":\"j1\",\"status\":\"SUCCESS\"},{\"id\":\"0ABC\",\"status\":\"SUCCESS\"}]}");

        List<DispatchJobsResource.BatchResult> results = client().dispatchJobs().createBatch(List.of(
                CreateDispatchJobDto.of("ingest:orders:record", "https://t.example.com/r")
                        .withId("j1").withClientCode("acme").withMetadata("source", "sftp")
                        .withPayload("a,b", "text/csv").withDataOnly(true),
                CreateDispatchJobDto.of("ingest:orders:record", "https://t.example.com/r")));

        assertEquals(2, results.size());
        assertEquals("j1", results.get(0).id());
        assertTrue(results.get(0).succeeded());

        JsonNode body = MAPPER.readTree(server.requests.getLast().body());
        JsonNode first = body.get("items").get(0);
        assertEquals("j1", first.get("id").asString());
        assertEquals("acme", first.get("clientCode").asString());
        assertEquals("[{\"key\":\"source\",\"value\":\"sftp\"}]", first.get("metadata").toString());
        assertEquals("text/csv", first.get("payloadContentType").asString());
        assertFalse(body.get("items").get(1).has("id"), "no id: the platform mints one");
    }

    @Test
    void anInvalidJobIsRefusedBeforeAnythingIsSent() {
        assertThrows(IllegalArgumentException.class, () -> client().dispatchJobs().createBatch(List.of(
                CreateDispatchJobDto.of("ingest:x", "https://t.example.com"),
                CreateDispatchJobDto.of("ingest:x", "ftp://t.example.com"))));
        assertTrue(server.requests.stream().noneMatch(r -> r.pathAndQuery().contains("dispatch-jobs")));
    }

    @Test
    void aWholeBatchRefusalIsAnException() {
        server.on("POST", "/api/dispatch-jobs/batch", 400,
                "{\"code\":\"CLIENT_NOT_FOUND\",\"message\":\"Client not found: nope\"}");
        FlowCatalystException e = assertThrows(FlowCatalystException.class, () -> client().dispatchJobs()
                .createBatch(List.of(CreateDispatchJobDto.of("ingest:x", "https://t.example.com").withClientCode("nope"))));
        assertTrue(e.getMessage().contains("Client not found"), e.getMessage());
    }

    @Test
    void anEmptyBatchSendsNothing() {
        assertEquals(List.of(), client().dispatchJobs().createBatch(List.of()));
        assertTrue(server.requests.stream().noneMatch(r -> r.pathAndQuery().contains("dispatch-jobs")));
    }
}
