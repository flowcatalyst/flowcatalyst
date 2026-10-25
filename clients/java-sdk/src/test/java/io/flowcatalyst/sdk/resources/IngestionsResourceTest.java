package io.flowcatalyst.sdk.resources;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.error.FlowCatalystException;
import io.flowcatalyst.sdk.error.SdkError;
import io.flowcatalyst.sdk.generated.model.ApplyIngestionSetRequest;
import io.flowcatalyst.sdk.generated.model.CreateIngestionRequest;
import io.flowcatalyst.sdk.generated.model.IngestionSetItem;
import io.flowcatalyst.sdk.generated.model.SaveIngestionVersionRequest;
import io.flowcatalyst.sdk.generated.model.TestIngestionRequest;
import io.flowcatalyst.sdk.generated.model.UpdateIngestionRequest;
import io.flowcatalyst.sdk.http.Json;
import io.flowcatalyst.sdk.resources.IngestionDefinitions.ApiAuth;
import io.flowcatalyst.sdk.resources.IngestionDefinitions.HttpAuth;
import io.flowcatalyst.sdk.resources.IngestionDefinitions.Mode;
import io.flowcatalyst.sdk.resources.IngestionDefinitions.Results;
import io.flowcatalyst.sdk.resources.IngestionDefinitions.SecretRef;
import io.flowcatalyst.sdk.resources.IngestionDefinitions.Source;
import io.flowcatalyst.sdk.resources.IngestionDefinitions.Split;
import io.flowcatalyst.sdk.resources.IngestionDefinitions.Track;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/** The ingestions and ingestion-sets resources, and the typed source/split forms. */
class IngestionsResourceTest {

    private static final ObjectMapper MAPPER = Json.newMapper();

    private static final String TARGET =
            "{\"code\":\"t\",\"targetUrl\":\"https://x.test/h\",\"pool\":\"p\",\"mode\":\"IMMEDIATE\",\"batchSize\":10}";

    private static final String INGESTION = "{\"id\":\"ing_1\",\"applicationId\":\"app_1\",\"applicationCode\":\"orders\","
            + "\"clientId\":\"clt_1\",\"clientIdentifier\":\"acme\",\"code\":\"feed\",\"status\":\"ACTIVE\","
            + "\"version\":1,\"latestVersion\":1,\"createdAt\":\"2026-10-24T10:00:00Z\","
            + "\"updatedAt\":\"2026-10-24T10:00:00Z\",\"name\":\"Feed\","
            + "\"source\":{\"kind\":\"http\",\"auth\":{\"kind\":\"bearer\",\"token\":{\"name\":\"feed-token\"}}},"
            + "\"split\":{\"kind\":\"csv\",\"header\":true},\"target\":" + TARGET + "}";

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

    private StubServer.Recorded last() {
        return server.requests.getLast();
    }

    private static Source.Http bearerSource() {
        return new Source.Http(new HttpAuth.Bearer(new SecretRef("feed-token")), null);
    }

    // ── Requests: method, path, query ───────────────────────────────

    @Test
    void listSendsNoQueryWhenNoFilterIsGivenAndOnlyTheGivenOnesOtherwise() {
        server.on("GET", "/api/ingestions", 200, "{\"ingestions\":[" + INGESTION + "],\"hasMore\":false}");

        var page = client().ingestions().list(null);
        assertEquals("GET", last().method());
        assertEquals("/api/ingestions", last().pathAndQuery());
        assertEquals("ing_1", page.getIngestions().getFirst().getId());

        client().ingestions().list(IngestionsResource.ListFilters.none());
        assertEquals("/api/ingestions", last().pathAndQuery());

        client().ingestions().list(new IngestionsResource.ListFilters("orders", "acme co", "fe ed", "PAUSED", "sftp", null, 50));
        assertEquals(
                "/api/ingestions?applicationCode=orders&clientId=acme+co&q=fe+ed&status=PAUSED&kind=sftp&limit=50",
                last().pathAndQuery());
    }

    @Test
    void listPagesFollowTheCursor() {
        server.on("GET", "/api/ingestions", r -> r.pathAndQuery().contains("after=")
                ? new StubServer.Reply(200, "{\"ingestions\":[],\"hasMore\":false}")
                : new StubServer.Reply(200,
                        "{\"ingestions\":[" + INGESTION + "],\"hasMore\":true,\"nextCursor\":\"cur/1 2\"}"));

        var ingestions = client().ingestions();
        var first = ingestions.list(IngestionsResource.ListFilters.page(null, 1));
        assertTrue(first.getHasMore());
        assertEquals("cur/1 2", first.getNextCursor());
        assertEquals("/api/ingestions?limit=1", last().pathAndQuery());

        var second = ingestions.list(IngestionsResource.ListFilters.page(first.getNextCursor(), 1));
        assertEquals("/api/ingestions?after=cur%2F1+2&limit=1", last().pathAndQuery());
        assertTrue(second.getIngestions().isEmpty());
        assertNull(second.getNextCursor());
    }

    @Test
    void createSendsTheTypedDefinitionAndReadsTheCreatedIngestion() throws Exception {
        server.on("POST", "/api/ingestions", 201, INGESTION);

        var created = client().ingestions().create(new CreateIngestionRequest()
                .application("orders")
                .client("acme")
                .code("feed")
                .name("Feed")
                .source(bearerSource())
                .split(new Split.Csv(true))
                .target(IngestionDefinitions.target("t", "https://x.test/h", "p", Mode.IMMEDIATE, 10)));

        assertEquals("POST", last().method());
        assertEquals("/api/ingestions", last().pathAndQuery());
        JsonNode body = MAPPER.readTree(last().body());
        assertEquals("orders", body.get("application").asText());
        assertEquals("acme", body.get("client").asText());
        assertEquals(
                MAPPER.readTree("{\"kind\":\"http\",\"auth\":{\"kind\":\"bearer\",\"token\":{\"name\":\"feed-token\"}}}"),
                body.get("source"));
        assertEquals(MAPPER.readTree("{\"kind\":\"csv\",\"header\":true}"), body.get("split"));
        assertEquals(MAPPER.readTree(TARGET), body.get("target"));
        assertEquals("ing_1", created.getId());
        assertEquals(bearerSource(), IngestionDefinitions.sourceOf(created.getSource()));
        assertEquals(new Split.Csv(true), IngestionDefinitions.splitOf(created.getSplit()));
    }

    @Test
    void getUpdateDeleteEncodeTheIdAndUseTheirMethods() {
        server.on("GET", "/api/ingestions/a b/c", 200, INGESTION);
        server.on("PUT", "/api/ingestions/a b/c", 200, INGESTION);
        server.on("DELETE", "/api/ingestions/a b/c", 204, null);

        var ingestions = client().ingestions();
        ingestions.get("a b/c");
        assertEquals("GET", last().method());
        assertEquals("/api/ingestions/a b/c", last().pathAndQuery());

        ingestions.update("a b/c", new UpdateIngestionRequest()
                .expectedVersion(3L).name("Feed").source(bearerSource()).split(new Split.None())
                .target(IngestionDefinitions.target("t", "https://x.test/h", "p", Mode.BLOCK_ON_ERROR, 1)));
        assertEquals("PUT", last().method());
        assertTrue(last().body().contains("\"expectedVersion\":3"), last().body());
        assertTrue(last().body().contains("\"mode\":\"BLOCK_ON_ERROR\""), last().body());

        ingestions.delete("a b/c");
        assertEquals("DELETE", last().method());
        assertEquals("/api/ingestions/a b/c", last().pathAndQuery());
    }

    @Test
    void pauseResumeAndVersions() {
        server.on("POST", "/api/ingestions/ing_1/pause", 200, INGESTION);
        server.on("POST", "/api/ingestions/ing_1/resume", 200, INGESTION);
        server.on("GET", "/api/ingestions/ing_1/versions", 200, "{\"versions\":[]}");
        server.on("POST", "/api/ingestions/ing_1/versions", 201,
                "{\"id\":\"v\",\"ingestionId\":\"ing_1\",\"version\":2,\"createdAt\":\"2026-10-24T10:00:00Z\","
                        + "\"name\":\"Feed\",\"source\":{},\"split\":{},\"target\":" + TARGET + "}");
        server.on("POST", "/api/ingestions/ing_1/versions/2/live", 200, INGESTION);

        var ingestions = client().ingestions();
        ingestions.pause("ing_1");
        assertEquals("/api/ingestions/ing_1/pause", last().pathAndQuery());
        ingestions.resume("ing_1");
        assertEquals("/api/ingestions/ing_1/resume", last().pathAndQuery());
        assertTrue(ingestions.listVersions("ing_1").getVersions().isEmpty());
        assertEquals("GET", last().method());

        var saved = ingestions.saveVersion("ing_1", new SaveIngestionVersionRequest()
                .name("Feed").live(false).source(bearerSource()).split(new Split.None())
                .target(IngestionDefinitions.target("t", "https://x.test/h", "p", Mode.NEXT_ON_ERROR, 5)));
        assertEquals("POST", last().method());
        assertEquals("/api/ingestions/ing_1/versions", last().pathAndQuery());
        assertEquals(2L, saved.getVersion());

        ingestions.makeVersionLive("ing_1", 2);
        assertEquals("/api/ingestions/ing_1/versions/2/live", last().pathAndQuery());
    }

    @Test
    void definitionIsTheYamlTextAsItCame() {
        String yaml = "application: orders\nclient: acme\ncode: feed\nactive: true\nname: Feed\n";
        server.on("GET", "/api/ingestions/ing_1/definition", 200, yaml);

        assertEquals(yaml, client().ingestions().getDefinition("ing_1"));
        assertEquals("/api/ingestions/ing_1/definition", last().pathAndQuery());
    }

    @Test
    void historyFailuresKindsAndTests() {
        server.on("GET", "/api/ingestions/ing_1/history", 200, "{\"entries\":[],\"hasMore\":false}");
        server.on("GET", "/api/ingestions/ing_1/failures", 200,
                "{\"failures\":[],\"hasMore\":true,\"nextCursor\":\"f9\"}");
        server.on("GET", "/api/ingestion-source-kinds", 200, "{\"kinds\":[]}");
        server.on("POST", "/api/ingestion-tests", 202,
                "{\"id\":\"tst_1\",\"kind\":\"sftp\",\"status\":\"PENDING\","
                        + "\"createdAt\":\"2026-10-24T10:00:00Z\",\"expiresAt\":\"2026-10-24T10:05:00Z\"}");
        server.on("GET", "/api/ingestion-tests/tst_1", 200,
                "{\"id\":\"tst_1\",\"kind\":\"sftp\",\"status\":\"DONE\",\"outcome\":\"RESULTS\",\"records\":3,"
                        + "\"createdAt\":\"2026-10-24T10:00:00Z\",\"expiresAt\":\"2026-10-24T10:05:00Z\"}");

        var ingestions = client().ingestions();
        ingestions.listHistory("ing_1", null, null);
        assertEquals("/api/ingestions/ing_1/history", last().pathAndQuery());
        ingestions.listHistory("ing_1", 20, 40);
        assertEquals("/api/ingestions/ing_1/history?limit=20&offset=40", last().pathAndQuery());

        var failures = ingestions.listFailures("ing_1", null, null);
        assertEquals("/api/ingestions/ing_1/failures", last().pathAndQuery());
        ingestions.listFailures("ing_1", failures.getNextCursor(), 10);
        assertEquals("/api/ingestions/ing_1/failures?after=f9&limit=10", last().pathAndQuery());

        assertTrue(ingestions.listSourceKinds().getKinds().isEmpty());

        var queued = ingestions.testConnection(new TestIngestionRequest().ingestionId("ing_1"));
        assertEquals("POST", last().method());
        assertEquals("{\"ingestionId\":\"ing_1\"}", last().body());
        assertEquals("PENDING", queued.getStatus().getValue());
        var done = ingestions.getTest(queued.getId());
        assertEquals("/api/ingestion-tests/tst_1", last().pathAndQuery());
        assertEquals(3L, done.getRecords());
    }

    @Test
    void failuresFilterByCategoryAndUndismissedOnlyWhenGiven() {
        server.on("GET", "/api/ingestions/ing_1/failures", 200, "{\"failures\":[],\"hasMore\":false}");

        var ingestions = client().ingestions();
        ingestions.listFailures("ing_1", IngestionsResource.FailureFilters.none());
        assertEquals("/api/ingestions/ing_1/failures", last().pathAndQuery());
        ingestions.listFailures("ing_1", IngestionsResource.FailureFilters.openNotices());
        assertEquals("/api/ingestions/ing_1/failures?category=rejected&undismissed=true", last().pathAndQuery());
        ingestions.listFailures("ing_1", new IngestionsResource.FailureFilters("c1", 5, "payload", false));
        assertEquals("/api/ingestions/ing_1/failures?after=c1&limit=5&category=payload", last().pathAndQuery());
    }

    @Test
    void dismissFailureReturnsTheDismissedFailure() {
        server.on("POST", "/api/ingestions/ing_1/failures/igf_1/dismiss", 200,
                "{\"id\":\"igf_1\",\"kind\":\"JOBS_REJECTED\",\"category\":\"rejected\",\"message\":\"refused\","
                        + "\"afterRecords\":0,\"occurredAt\":\"2026-10-25T10:00:00Z\",\"createdAt\":\"2026-10-25T10:00:01Z\","
                        + "\"dismissedAt\":\"2026-10-25T11:00:00Z\",\"dismissedBy\":\"prn_1\"}");

        var failure = client().ingestions().dismissFailure("ing_1", "igf_1");
        assertEquals("POST", last().method());
        assertEquals("/api/ingestions/ing_1/failures/igf_1/dismiss", last().pathAndQuery());
        assertEquals("rejected", failure.getCategory().getValue());
        assertEquals("prn_1", failure.getDismissedBy());
        assertEquals(11, failure.getDismissedAt().getHour());
    }

    @Test
    void secretsAreNamesAndFetchesOnly() {
        server.on("GET", "/api/ingestions/ing_1/secrets", 200,
                "{\"secrets\":[{\"name\":\"feed-token\",\"bound\":true,\"fetches\":[{\"runtimeId\":\"rt-1\","
                        + "\"version\":3,\"principalId\":\"prn_rt\",\"lastFetchedAt\":\"2026-10-25T10:00:00Z\","
                        + "\"fetchCount\":7}]}]}");

        var secrets = client().ingestions().listSecrets("ing_1");
        assertEquals("/api/ingestions/ing_1/secrets", last().pathAndQuery());
        var secret = secrets.getSecrets().getFirst();
        assertEquals("feed-token", secret.getName());
        assertTrue(secret.getBound());
        assertEquals(7L, secret.getFetches().getFirst().getFetchCount());
    }

    @Test
    void aCreateWithoutPromoteAnswersADraftWithNoticesAndWarnings() {
        String answer = INGESTION.substring(0, INGESTION.length() - 1)
                + ",\"notices\":2,\"latestNotice\":\"jobs refused\",\"latestNoticeAt\":\"2026-10-25T10:00:00Z\","
                + "\"warnings\":[\"no host key\"],"
                + "\"save\":{\"outcome\":\"DRAFT\",\"version\":1,\"awaitingPromote\":true,\"message\":\"saved as a draft\"}}";
        server.on("POST", "/api/ingestions", 201, answer);
        server.on("GET", "/api/ingestions/ing_1/versions", 200,
                "{\"versions\":[{\"id\":\"igv_1\",\"version\":1,\"live\":false,\"draft\":true,\"digest\":\"ab12\","
                        + "\"createdAt\":\"2026-10-25T10:00:00Z\",\"name\":\"Feed\",\"source\":{\"kind\":\"http\"},"
                        + "\"split\":{\"kind\":\"none\"},\"target\":" + TARGET + "}]}");

        var ing = client().ingestions().create(new CreateIngestionRequest().code("feed"));
        assertEquals("DRAFT", ing.getSave().getOutcome().getValue());
        assertTrue(ing.getSave().getAwaitingPromote());
        assertEquals(2L, ing.getNotices());
        assertEquals("jobs refused", ing.getLatestNotice());
        assertEquals(List.of("no host key"), ing.getWarnings());

        var version = client().ingestions().listVersions("ing_1").getVersions().getFirst();
        assertTrue(version.getDraft());
        assertEquals("ab12", version.getDigest());
    }

    // ── Errors ──────────────────────────────────────────────────────

    @Test
    void errorStatusesMapToTheNormalErrorTypes() {
        server.on("POST", "/api/ingestions", r -> r.body().contains("\"code\":\"dup\"")
                ? new StubServer.Reply(409, "{\"error\":\"INGESTION_CODE_EXISTS\",\"message\":\"code exists\"}")
                : new StubServer.Reply(400, "{\"error\":\"INGESTION_TARGET_INVALID\",\"message\":\"pool not found\"}"));
        server.on("GET", "/api/ingestions/forbidden", 403, "{\"error\":\"FORBIDDEN\",\"message\":\"no\"}");
        server.on("GET", "/api/ingestions/missing", 404,
                "{\"error\":\"INGESTION_NOT_FOUND\",\"message\":\"none\"}");

        var ingestions = client().ingestions();
        var conflict = assertInstanceOf(SdkError.Conflict.class, assertThrows(FlowCatalystException.class,
                () -> ingestions.create(new CreateIngestionRequest().code("dup"))).error());
        assertEquals("INGESTION_CODE_EXISTS", conflict.code());

        var invalid = assertInstanceOf(SdkError.HttpStatus.class, assertThrows(FlowCatalystException.class,
                () -> ingestions.create(new CreateIngestionRequest().code("ok"))).error());
        assertEquals(400, invalid.status());
        assertTrue(invalid.body().contains("INGESTION_TARGET_INVALID"));

        assertInstanceOf(SdkError.Forbidden.class,
                assertThrows(FlowCatalystException.class, () -> ingestions.get("forbidden")).error());
        assertInstanceOf(SdkError.NotFound.class,
                assertThrows(FlowCatalystException.class, () -> ingestions.get("missing")).error());
    }

    // ── Sets ────────────────────────────────────────────────────────

    private static ApplyIngestionSetRequest setRequest() {
        return new ApplyIngestionSetRequest().addItemsItem(new IngestionSetItem()
                .application("orders").client("acme").code("feed").name("Feed")
                .source(bearerSource()).split(new Split.None())
                .target(IngestionDefinitions.target("t", "https://x.test/h", "p", Mode.IMMEDIATE, 10)));
    }

    @Test
    void applySendsDryRunOnlyWhenGiven() {
        server.on("PUT", "/api/ingestion-sets/my set", 200, "{\"name\":\"my set\",\"dryRun\":false,\"changes\":[]}");

        var sets = client().ingestionSets();
        sets.apply("my set", setRequest(), null);
        assertEquals("PUT", last().method());
        assertEquals("/api/ingestion-sets/my set", last().pathAndQuery());
        assertTrue(last().body().contains("\"items\":["), last().body());

        sets.apply("my set", setRequest(), true);
        assertEquals("/api/ingestion-sets/my set?dryRun=true", last().pathAndQuery());
        sets.apply("my set", setRequest(), false);
        assertEquals("/api/ingestion-sets/my set?dryRun=false", last().pathAndQuery());
    }

    @Test
    void setListsAppliesReapplyAndRelease() {
        server.on("GET", "/api/ingestion-sets", 200, "{\"sets\":[]}");
        server.on("GET", "/api/ingestion-sets/s/applies", 200, "{\"applies\":[]}");
        server.on("GET", "/api/ingestion-sets/s/applies/ap_1", 200,
                "{\"set\":\"s\",\"applyId\":\"ap_1\",\"appliedAt\":\"2026-10-24T10:00:00Z\",\"appliedBy\":\"u\","
                        + "\"items\":[],\"files\":[]}");
        server.on("POST", "/api/ingestion-sets/s/applies/ap_1/reapply", 200,
                "{\"name\":\"s\",\"dryRun\":true,\"changes\":[]}");
        server.on("POST", "/api/ingestion-sets/s/release", 200, "{\"name\":\"s\",\"released\":2}");
        server.on("PUT", "/api/ingestion-sets/owned", 409,
                "{\"error\":\"INGESTION_SET_OWNED\",\"message\":\"owned by another set\"}");

        var sets = client().ingestionSets();
        assertTrue(sets.list().getSets().isEmpty());
        assertEquals("/api/ingestion-sets", last().pathAndQuery());
        assertTrue(sets.listApplies("s").getApplies().isEmpty());
        assertEquals("ap_1", sets.getApply("s", "ap_1").getApplyId());
        assertEquals("/api/ingestion-sets/s/applies/ap_1", last().pathAndQuery());

        sets.reapply("s", "ap_1", null);
        assertEquals("POST", last().method());
        assertEquals("/api/ingestion-sets/s/applies/ap_1/reapply", last().pathAndQuery());
        sets.reapply("s", "ap_1", true);
        assertEquals("/api/ingestion-sets/s/applies/ap_1/reapply?dryRun=true", last().pathAndQuery());

        sets.release("s");
        assertEquals("POST", last().method());
        assertEquals("/api/ingestion-sets/s/release", last().pathAndQuery());

        var owned = assertInstanceOf(SdkError.Conflict.class, assertThrows(FlowCatalystException.class,
                () -> sets.apply("owned", setRequest(), null)).error());
        assertEquals("INGESTION_SET_OWNED", owned.code());
    }

    // ── Typed forms: wire shape and round trip ──────────────────────

    private static void roundTrips(Object typed, String wire, Class<?> union) throws Exception {
        assertEquals(MAPPER.readTree(wire), MAPPER.readTree(MAPPER.writeValueAsString(typed)), wire);
        assertEquals(typed, MAPPER.readValue(wire, union), wire);
    }

    @Test
    void everySourceKindRoundTripsThroughItsWireForm() throws Exception {
        var secret = "{\"name\":\"s\"}";
        roundTrips(new Source.Http(new HttpAuth.FlowCatalystToken("orders", null), null),
                "{\"kind\":\"http\",\"auth\":{\"kind\":\"flowcatalyst-token\",\"application\":\"orders\"}}", Source.class);
        roundTrips(new Source.Http(new HttpAuth.Basic("u", new SecretRef("s")),
                        new IngestionDefinitions.RateLimit(100, Duration.ofMinutes(1))),
                "{\"kind\":\"http\",\"auth\":{\"kind\":\"basic\",\"username\":\"u\",\"password\":" + secret + "},"
                        + "\"rateLimit\":{\"requests\":100,\"per\":\"PT1M\"}}", Source.class);
        roundTrips(new Source.Sftp("h", 22, "/in", "u", new SecretRef("s"), null, "*.csv", Duration.ofSeconds(30)),
                "{\"kind\":\"sftp\",\"host\":\"h\",\"port\":22,\"directory\":\"/in\",\"username\":\"u\","
                        + "\"credential\":" + secret + ",\"include\":\"*.csv\",\"every\":\"PT30S\"}", Source.class);
        roundTrips(new Source.Sftp("h", 22, "/in", "u", new SecretRef("s"), null, true, null, Duration.ofMinutes(5)),
                "{\"kind\":\"sftp\",\"host\":\"h\",\"port\":22,\"directory\":\"/in\",\"username\":\"u\","
                        + "\"credential\":" + secret + ",\"acceptAnyHostKey\":true,\"every\":\"PT5M\"}", Source.class);
        roundTrips(new Source.S3("eu-west-1", "b", "in/", Duration.ofMinutes(5), null),
                "{\"kind\":\"s3\",\"region\":\"eu-west-1\",\"bucket\":\"b\",\"prefix\":\"in/\",\"every\":\"PT5M\"}",
                Source.class);
        roundTrips(new Source.Sqs("eu-west-1", "q", new IngestionDefinitions.AwsCredential("AK", new SecretRef("s"))),
                "{\"kind\":\"sqs\",\"region\":\"eu-west-1\",\"queue\":\"q\","
                        + "\"credential\":{\"accessKeyId\":\"AK\",\"secretAccessKey\":" + secret + "}}", Source.class);
        roundTrips(new Source.RabbitMq("amqps://h", "q", "u", new SecretRef("s")),
                "{\"kind\":\"rabbitmq\",\"uri\":\"amqps://h\",\"queue\":\"q\",\"username\":\"u\",\"password\":" + secret + "}",
                Source.class);
        roundTrips(new Source.DatabasePoll("jdbc:postgresql://h/d", "u", new SecretRef("s"), "t",
                        new Track.Watermark("id", "NUMBER"), 500, Duration.ofSeconds(10)),
                "{\"kind\":\"database-poll\",\"jdbcUrl\":\"jdbc:postgresql://h/d\",\"username\":\"u\",\"password\":" + secret
                        + ",\"table\":\"t\",\"track\":{\"kind\":\"watermark\",\"column\":\"id\",\"type\":\"NUMBER\"},"
                        + "\"maxRows\":500,\"every\":\"PT10S\"}", Source.class);
        roundTrips(new Source.ApiPoll("https://a.test/x", "GET",
                        List.of(new IngestionDefinitions.Header("Accept", "application/json")), null,
                        new ApiAuth.None(), new Results.JsonPath("$.items"),
                        new IngestionDefinitions.Cursor("$.next", "after", null),
                        Duration.ofSeconds(5), Duration.ofMinutes(1)),
                "{\"kind\":\"api-poll\",\"url\":\"https://a.test/x\",\"method\":\"GET\","
                        + "\"headers\":[{\"name\":\"Accept\",\"value\":\"application/json\"}],"
                        + "\"auth\":{\"kind\":\"none\"},\"results\":{\"kind\":\"json-path\",\"path\":\"$.items\"},"
                        + "\"cursor\":{\"from\":\"$.next\",\"param\":\"after\"},\"every\":\"PT5S\",\"idleDelay\":\"PT1M\"}",
                Source.class);
    }

    @Test
    void customKindCarriesItsTypeAndOpenSettings() throws Exception {
        roundTrips(new Source.Custom("camel-route", Map.of("yaml", "- from: {uri: timer:t}")),
                "{\"kind\":\"custom\",\"type\":\"camel-route\",\"settings\":{\"yaml\":\"- from: {uri: timer:t}\"}}",
                Source.class);
    }

    @Test
    void everySplitRoundTrips() throws Exception {
        roundTrips(new Split.None(), "{\"kind\":\"none\"}", Split.class);
        roundTrips(new Split.Csv(false), "{\"kind\":\"csv\",\"header\":false}", Split.class);
        roundTrips(new Split.Json("$.rows"), "{\"kind\":\"json\",\"arrayPath\":\"$.rows\"}", Split.class);
        roundTrips(new Split.Xml("row"), "{\"kind\":\"xml\",\"element\":\"row\"}", Split.class);
    }

    @Test
    void aSecretIsOnlyEverItsName() throws Exception {
        String wire = MAPPER.writeValueAsString(new ApiAuth.Basic("u", new SecretRef("db-password")));
        assertEquals(MAPPER.readTree("{\"kind\":\"basic\",\"username\":\"u\",\"password\":{\"name\":\"db-password\"}}"),
                MAPPER.readTree(wire));
    }

    @Test
    void targetModeNamesMatchTheWire() {
        assertEquals(List.of("IMMEDIATE", "NEXT_ON_ERROR", "BLOCK_ON_ERROR"),
                List.of(Mode.values()).stream().map(Enum::name).toList());
    }
}
