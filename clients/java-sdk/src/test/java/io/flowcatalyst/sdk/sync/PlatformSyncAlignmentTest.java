package io.flowcatalyst.sdk.sync;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.sync.Definitions.DefinitionSet;
import io.flowcatalyst.sdk.sync.SyncResult.Category;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/**
 * The application sync as the platform answers it today (api/openapi.lock.json;
 * docs/platform-bug-fixes.md F13, F15): the principal sync's {@code skippedRoles} and
 * {@code passwordHashIgnored}, a principal's {@code passwordHash}, the documentation category,
 * and the dispatch-pool rules.
 */
class PlatformSyncAlignmentTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final String SYNC_OK =
            "{\"applicationCode\":\"orders\",\"created\":1,\"updated\":0,\"deleted\":0,\"syncedCodes\":[\"x\"]}";

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
                .build();
    }

    @Test
    void thePrincipalSyncReportsSkippedRolesAndIgnoredPasswordHashes() {
        server.on("POST", "/api/applications/orders/principals/sync", 200,
                "{\"applicationCode\":\"orders\",\"created\":1,\"updated\":1,\"deleted\":0,"
                        + "\"syncedCodes\":[\"a@x.io\",\"b@x.io\"],\"skippedRoles\":[\"orders:typo\"],"
                        + "\"passwordHashIgnored\":[\"b@x.io\"]}");

        SyncResult result = client().definitions().sync(DefinitionSet.define("orders")
                .withPrincipals(List.of(
                        Definitions.Principal.of("a@x.io", "A").withRoles(List.of("orders:admin", "orders:typo")),
                        Definitions.Principal.of("b@x.io", "B").withPasswordHash("$2y$10$abc"))));

        Category.Synced principals = assertInstanceOf(Category.Synced.class, result.principals());
        assertEquals(List.of("orders:typo"), principals.skippedRoles());
        assertEquals(List.of("b@x.io"), principals.passwordHashIgnored());

        JsonNode body = MAPPER.readTree(server.requests.getLast().body());
        assertEquals("$2y$10$abc", body.get("principals").get(1).get("passwordHash").asString());
        assertFalse(body.get("principals").get(0).has("passwordHash"));
    }

    @Test
    void anOmittedReportIsEmptyNotNull() {
        server.on("POST", "/api/applications/orders/roles/sync", 200, SYNC_OK);
        SyncResult result = client().definitions().sync(DefinitionSet.define("orders")
                .withRoles(List.of(Definitions.Role.of("admin"))));
        Category.Synced roles = assertInstanceOf(Category.Synced.class, result.roles());
        assertEquals(List.of(), roles.skippedRoles());
        assertEquals(List.of(), roles.passwordHashIgnored());
        assertEquals(new Category.Synced("orders", 1, 0, 0, List.of("x")), roles,
                "the five-argument constructor still builds the same result");
    }

    @Test
    void docsSyncAsOneDeclarativeSet() {
        server.on("POST", "/api/applications/orders/docs/sync", 200,
                "{\"applicationCode\":\"orders\",\"created\":2,\"updated\":0,\"deleted\":1,"
                        + "\"syncedCodes\":[\"getting-started\",\"events\"]}");

        SyncResult result = client().definitions().sync(DefinitionSet.define("orders")
                .withDocs(List.of(
                        Definitions.Doc.of("getting-started", "# Getting started\n\nHello."),
                        Definitions.Doc.of("events", "Events.").withTitle("Events"))),
                SyncOptions.removingUnlisted());

        Category.Synced docs = assertInstanceOf(Category.Synced.class, result.docs());
        assertEquals(2, docs.created());
        assertEquals(1, docs.deleted());

        StubServer.Recorded call = server.requests.getLast();
        assertEquals("/api/applications/orders/docs/sync", call.pathAndQuery(),
                "the docs sync takes no removeUnlisted: the payload is the whole set");
        JsonNode body = MAPPER.readTree(call.body());
        assertEquals("getting-started", body.get("docs").get(0).get("slug").asString());
        assertFalse(body.get("docs").get(0).has("title"));
        assertEquals("Events", body.get("docs").get(1).get("title").asString());
    }

    @Test
    void docsAreSkippedWhenNoneAreDeclaredOrTheCategoryIsSkipped() {
        server.on("POST", "/api/applications/orders/roles/sync", 200, SYNC_OK);
        SyncResult none = client().definitions().sync(DefinitionSet.define("orders")
                .withRoles(List.of(Definitions.Role.of("admin"))));
        assertEquals(Category.SKIPPED, none.docs());

        SyncResult skipped = client().definitions().sync(DefinitionSet.define("orders")
                        .withDocs(List.of(Definitions.Doc.of("a", "x"))),
                SyncOptions.defaults().skipping(SyncOptions.SyncCategory.DOCS));
        assertEquals(Category.SKIPPED, skipped.docs());
        assertTrue(server.requests.stream().noneMatch(r -> r.pathAndQuery().contains("/docs/")));
    }

    @Test
    void aDuplicateDocSlugFailsLocally() {
        DefinitionSyncException e = assertThrows(DefinitionSyncException.class, () ->
                client().definitions().syncGrouped(List.of(
                        DefinitionSet.define("orders").withDocs(List.of(Definitions.Doc.of("a", "x"))),
                        DefinitionSet.define("orders").withDocs(List.of(Definitions.Doc.of("a", "y"))))));
        assertInstanceOf(Category.Failed.class, e.resultsByApplication().get("orders").docs());
        assertTrue(e.getMessage().contains("docs"), e.getMessage());
        assertTrue(server.requests.stream().noneMatch(r -> r.pathAndQuery().contains("/docs/")));
    }

    @Test
    void theTenArgumentSyncResultStillBuilds() {
        SyncResult r = new SyncResult("orders", Category.SKIPPED, Category.SKIPPED, Category.SKIPPED,
                Category.SKIPPED, Category.SKIPPED, Category.SKIPPED, Category.SKIPPED, Category.SKIPPED,
                Category.SKIPPED);
        assertEquals(Category.SKIPPED, r.docs());
    }

    @Test
    void aDispatchPoolRateLimitOrConcurrencyBelowOneIsRefusedLocally() {
        assertThrows(IllegalArgumentException.class, () -> Definitions.DispatchPool.of("p", "P").withRateLimit(0));
        assertThrows(IllegalArgumentException.class, () -> Definitions.DispatchPool.of("p", "P").withConcurrency(0));
        assertEquals(null, Definitions.DispatchPool.of("p", "P").withRateLimit(5).withoutRateLimit().rateLimit());
    }

    @Test
    void theFourArgumentPrincipalStillBuilds() {
        Definitions.Principal p = new Definitions.Principal("a@x.io", "A", List.of("orders:admin"), true);
        assertEquals(null, p.passwordHash());
    }
}
