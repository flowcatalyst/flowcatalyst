package io.flowcatalyst.sdk.sync;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.annotations.AsConnection;
import io.flowcatalyst.sdk.annotations.DefinitionScanner;
import io.flowcatalyst.sdk.sync.Definitions.Connection;
import io.flowcatalyst.sdk.sync.Definitions.DefinitionSet;
import io.flowcatalyst.sdk.sync.Definitions.EventType;
import io.flowcatalyst.sdk.sync.Definitions.Role;
import io.flowcatalyst.sdk.sync.Definitions.Subscription;
import io.flowcatalyst.sdk.sync.Definitions.SubscriptionEventType;
import io.flowcatalyst.sdk.sync.SyncResult.Category;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/**
 * {@link DefinitionSynchronizer#syncGrouped} — merging every set sharing an
 * application code into ONE sync per (application, client) scope, so that
 * two sets contributing to the same scope never become two platform calls
 * (the second of which would delete what the first just created under
 * {@code removeUnlisted}).
 */
class MergedSyncTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final String SYNC_OK =
            "{\"applicationCode\":\"orders\",\"created\":1,\"updated\":0,\"deleted\":0,"
                    + "\"syncedCodes\":[\"x\"]}";

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

    private List<StubServer.Recorded> callsTo(String suffix) {
        return server.requests.stream().filter(r -> r.pathAndQuery().contains(suffix)).toList();
    }

    /**
     * THE critical regression test: two sets for the same application, both
     * defining connections for the SAME scope (first: both global; then:
     * both bound to the same client). removeUnlisted=true must still yield
     * exactly ONE connections request and ONE subscriptions request per
     * scope, containing BOTH sets' definitions — not two separate requests
     * where the second would wipe out what the first just created.
     */
    @Test
    void twoSetsForTheSameScopeMergeIntoOneCallEach() throws Exception {
        server.on("POST", "/api/applications/orders/connections/sync", 200, SYNC_OK);
        server.on("POST", "/api/applications/orders/subscriptions/sync", 200, SYNC_OK);

        DefinitionSet setA = DefinitionSet.define("orders")
                .withConnections(List.of(Connection.of("conn-a", "Connection A")))
                .withSubscriptions(List.of(Subscription.of(
                        "sub-a", "Sub A", "https://a.example.com/hook",
                        List.of(SubscriptionEventType.of("orders:sales:order:created")))
                        .withConnectionCode("conn-a")));
        DefinitionSet setB = DefinitionSet.define("orders")
                .withConnections(List.of(Connection.of("conn-b", "Connection B")))
                .withSubscriptions(List.of(Subscription.of(
                        "sub-b", "Sub B", "https://b.example.com/hook",
                        List.of(SubscriptionEventType.of("orders:sales:order:created")))
                        .withConnectionCode("conn-b")));

        client().definitions().syncGrouped(List.of(setA, setB), SyncOptions.removingUnlisted());

        List<StubServer.Recorded> connectionCalls = callsTo("connections/sync");
        List<StubServer.Recorded> subscriptionCalls = callsTo("subscriptions/sync");
        assertEquals(1, connectionCalls.size(), "exactly one connections call for the global scope");
        assertEquals(1, subscriptionCalls.size(), "exactly one subscriptions call for the global scope");

        JsonNode connBody = MAPPER.readTree(connectionCalls.get(0).body());
        assertEquals(2, connBody.get("connections").size(), "both sets' connections in one request");

        JsonNode subBody = MAPPER.readTree(subscriptionCalls.get(0).body());
        assertEquals(2, subBody.get("subscriptions").size(), "both sets' subscriptions in one request");
    }

    /** Same regression, but both sets bound to the SAME client via forClient(). */
    @Test
    void twoSetsForTheSameClientScopeMergeIntoOneCallEach() throws Exception {
        server.on("POST", "/api/applications/orders/connections/sync", 200, SYNC_OK);

        DefinitionSet setA = DefinitionSet.define("orders")
                .forClient("acme")
                .withConnections(List.of(Connection.of("conn-a", "Connection A")));
        DefinitionSet setB = DefinitionSet.define("orders")
                .forClient("acme")
                .withConnections(List.of(Connection.of("conn-b", "Connection B")));

        client().definitions().syncGrouped(List.of(setA, setB), SyncOptions.removingUnlisted());

        List<StubServer.Recorded> connectionCalls = callsTo("connections/sync");
        assertEquals(1, connectionCalls.size(), "exactly one connections call for client acme");
        JsonNode body = MAPPER.readTree(connectionCalls.get(0).body());
        assertEquals("acme", body.get("clientId").asText());
        assertEquals(2, body.get("connections").size());
    }

    /** Merging applies to every per-application type, not just connections/subscriptions. */
    @Test
    void mergingAlsoAppliesToRolesAndEventTypes() throws Exception {
        server.on("POST", "/api/applications/orders/roles/sync", 200, SYNC_OK);
        server.on("POST", "/api/applications/orders/event-types/sync", 200, SYNC_OK);

        DefinitionSet setA = DefinitionSet.define("orders")
                .withRoles(List.of(Role.of("admin")))
                .withEventTypes(List.of(EventType.of("orders:sales:order:created", "Order Created")));
        DefinitionSet setB = DefinitionSet.define("orders")
                .withRoles(List.of(Role.of("editor")))
                .withEventTypes(List.of(EventType.of("orders:sales:order:shipped", "Order Shipped")));

        client().definitions().syncGrouped(List.of(setA, setB));

        assertEquals(1, callsTo("roles/sync").size());
        assertEquals(1, callsTo("event-types/sync").size());

        JsonNode roleBody = MAPPER.readTree(callsTo("roles/sync").get(0).body());
        assertEquals(2, roleBody.get("roles").size());
    }

    /** global set + client A set + second client A set + client B set → order global, A (merged), B. */
    @Test
    void mixedSetsOrderGlobalThenEachClientMergedInFirstSeenOrder() throws Exception {
        server.on("POST", "/api/applications/orders/connections/sync", 200, SYNC_OK);

        DefinitionSet global = DefinitionSet.define("orders")
                .withConnections(List.of(Connection.of("conn-global", "Global")));
        DefinitionSet clientA1 = DefinitionSet.define("orders")
                .forClient("acme")
                .withConnections(List.of(Connection.of("conn-a1", "A1")));
        DefinitionSet clientA2 = DefinitionSet.define("orders")
                .forClient("acme")
                .withConnections(List.of(Connection.of("conn-a2", "A2")));
        DefinitionSet clientB = DefinitionSet.define("orders")
                .forClient("beta")
                .withConnections(List.of(Connection.of("conn-b", "B")));

        client().definitions().syncGrouped(List.of(global, clientA1, clientA2, clientB));

        List<StubServer.Recorded> calls = callsTo("connections/sync");
        assertEquals(3, calls.size(), "global, acme(merged), beta");

        JsonNode call0 = MAPPER.readTree(calls.get(0).body());
        assertFalse(call0.has("clientId"), "global first");

        JsonNode call1 = MAPPER.readTree(calls.get(1).body());
        assertEquals("acme", call1.get("clientId").asText());
        assertEquals(2, call1.get("connections").size(), "acme's two sets merged");

        JsonNode call2 = MAPPER.readTree(calls.get(2).body());
        assertEquals("beta", call2.get("clientId").asText());
    }

    /** Duplicate code across two merged sets in the same scope fails locally; other types still sync. */
    @Test
    void duplicateCodeAcrossMergedSetsFailsLocallyWithoutBlockingOtherTypes() throws Exception {
        server.on("POST", "/api/applications/orders/roles/sync", 200, SYNC_OK);

        DefinitionSet setA = DefinitionSet.define("orders")
                .withConnections(List.of(Connection.of("dup", "First")))
                .withRoles(List.of(Role.of("admin")));
        DefinitionSet setB = DefinitionSet.define("orders")
                .withConnections(List.of(Connection.of("dup", "Second")));

        SyncResult result = client().definitions().syncGrouped(List.of(setA, setB)).get("orders");

        Category.Failed connections = assertInstanceOf(Category.Failed.class, result.connections());
        assertTrue(connections.error().contains("dup"), "names the code");
        assertTrue(connections.error().contains("orders"), "names the scope");
        assertTrue(callsTo("connections/sync").isEmpty(), "no request sent for the failing type");

        assertInstanceOf(Category.Synced.class, result.roles());
        assertEquals(1, callsTo("roles/sync").size(), "other types still sync");
    }

    /** Per-set targetBaseUrl wins per row after merging; connectionCode-only sets don't need one. */
    @Test
    void perSetTargetBaseUrlAppliesAfterMerging() throws Exception {
        server.on("POST", "/api/applications/orders/subscriptions/sync", 200, SYNC_OK);

        DefinitionSet tenantSet = DefinitionSet.define("orders")
                .forClient("acme", "https://acme.example.com")
                .withSubscriptions(List.of(Subscription.of(
                        "sub-a", "Sub A", "/webhooks/orders",
                        List.of(SubscriptionEventType.of("orders:sales:order:created")))
                        .withConnectionCode("conn-a")));

        client().definitions().syncGrouped(List.of(tenantSet));

        JsonNode entry = MAPPER.readTree(callsTo("subscriptions/sync").get(0).body())
                .get("subscriptions").get(0);
        assertEquals("https://acme.example.com/webhooks/orders", entry.get("target").asText());
        assertFalse(entry.has("targetBaseUrl"), "the internal per-set base carrier never reaches the wire");
        assertFalse(entry.has("_targetBaseUrl"), "the internal per-set base carrier never reaches the wire");
    }

    /** {@code SyncOptions.skipping(CONNECTIONS)} force-skips connections even when present. */
    @Test
    void skippingConnectionsCategoryForceSkipsEvenWhenPresent() {
        DefinitionSet set = DefinitionSet.define("orders")
                .withConnections(List.of(Connection.of("conn-a", "A")));

        SyncResult result = client().definitions().syncGrouped(
                List.of(set), SyncOptions.defaults().skipping(SyncOptions.SyncCategory.CONNECTIONS))
                .get("orders");

        assertEquals(Category.SKIPPED, result.connections());
        assertTrue(callsTo("connections/sync").isEmpty());
    }

    /** client never appears inside a posted entry; sharedConnection only when true. */
    @Test
    void clientNeverAppearsInsidePostedEntries() throws Exception {
        server.on("POST", "/api/applications/orders/connections/sync", 200, SYNC_OK);
        server.on("POST", "/api/applications/orders/subscriptions/sync", 200, SYNC_OK);

        DefinitionSet set = DefinitionSet.define("orders")
                .forClient("acme")
                .withConnections(List.of(Connection.of("conn-a", "A")))
                .withSubscriptions(List.of(Subscription.of(
                        "sub-a", "Sub A", "https://a.example.com/hook",
                        List.of(SubscriptionEventType.of("orders:sales:order:created")))
                        .withConnectionCode("conn-a")
                        .withSharedConnection(false)));

        client().definitions().sync(set, SyncOptions.removingUnlisted());

        JsonNode connBody = MAPPER.readTree(callsTo("connections/sync").get(0).body());
        assertFalse(connBody.get("connections").get(0).has("client"));

        JsonNode subBody = MAPPER.readTree(callsTo("subscriptions/sync").get(0).body());
        JsonNode subEntry = subBody.get("subscriptions").get(0);
        assertFalse(subEntry.has("client"));
        assertFalse(subEntry.has("sharedConnection"), "sharedConnection omitted when false");
    }

    @Test
    void sharedConnectionSentOnlyWhenTrue() throws Exception {
        server.on("POST", "/api/applications/orders/subscriptions/sync", 200, SYNC_OK);

        DefinitionSet set = DefinitionSet.define("orders")
                .withSubscriptions(List.of(Subscription.of(
                        "sub-a", "Sub A", "https://a.example.com/hook",
                        List.of(SubscriptionEventType.of("orders:sales:order:created")))
                        .withConnectionCode("shared-conn")
                        .withSharedConnection(true)));

        client().definitions().sync(set);

        JsonNode body = MAPPER.readTree(callsTo("subscriptions/sync").get(0).body());
        assertTrue(body.get("subscriptions").get(0).get("sharedConnection").asBoolean());
    }

    // ── @AsConnection scanning + client precedence ──────────────────

    @AsConnection(code = "scanned-conn", name = "Scanned Connection")
    static final class NoClientConnection {}

    @AsConnection(code = "scanned-conn-2", name = "Scanned Connection 2", client = "acme")
    static final class AnnotatedClientConnection {}

    @Test
    void scannerBuildsConnectionsFromAnnotatedClasses() {
        DefinitionSet set = DefinitionScanner.scan("orders", List.of(NoClientConnection.class));
        Connection connection = set.connections().getFirst();
        assertEquals("scanned-conn", connection.code());
        assertEquals("Scanned Connection", connection.name());
    }

    @Test
    void annotationClientBeatsConfiguredDefault() {
        DefinitionSet set = DefinitionScanner.scan(
                "orders", List.of(AnnotatedClientConnection.class), "default-client");
        assertEquals("acme", set.connections().getFirst().client());
    }

    @Test
    void configuredDefaultAppliesWhenAnnotationHasNoClient() {
        DefinitionSet set = DefinitionScanner.scan(
                "orders", List.of(NoClientConnection.class), "default-client");
        assertEquals("default-client", set.connections().getFirst().client());
    }

    @Test
    void noClientAtAllWhenNeitherAnnotationNorDefaultSetsOne() {
        DefinitionSet set = DefinitionScanner.scan("orders", List.of(NoClientConnection.class));
        assertEquals(null, set.connections().getFirst().client());
    }

    // ── source compatibility ─────────────────────────────────────────

    @Test
    void subscriptionOldConstructorsStillCompileAndWork() {
        // The pre-connectionCode (11-arg) constructor.
        Subscription legacy = new Subscription(
                "code", "name", "desc", "https://example.com", "conn-id",
                List.of(SubscriptionEventType.of("orders:sales:order:created")), "pool",
                Definitions.SubscriptionMode.IMMEDIATE, 3, 30, true);
        assertEquals("code", legacy.code());
        assertEquals(null, legacy.connectionCode());
        assertEquals(null, legacy.client());
        assertEquals(null, legacy.sharedConnection());

        // The post-bb1b483 (12-arg, with connectionCode) constructor.
        Subscription withCode = new Subscription(
                "code", "name", "desc", "https://example.com", "conn-id",
                List.of(SubscriptionEventType.of("orders:sales:order:created")), "pool",
                Definitions.SubscriptionMode.IMMEDIATE, 3, 30, true, "conn-code");
        assertEquals("conn-code", withCode.connectionCode());
        assertEquals(null, withCode.client());
    }

    // ── connection sync failure skips subscriptions for that scope ────

    @Test
    void connectionSyncFailureSkipsSubscriptionsForThatScopeOnly() throws Exception {
        server.on("POST", "/api/applications/orders/connections/sync", 500,
                "{\"error\":\"INTERNAL\",\"message\":\"boom\"}");

        DefinitionSet set = DefinitionSet.define("orders")
                .withConnections(List.of(Connection.of("conn-a", "A")))
                .withSubscriptions(List.of(Subscription.of(
                        "sub-a", "Sub A", "https://a.example.com/hook",
                        List.of(SubscriptionEventType.of("orders:sales:order:created")))
                        .withConnectionCode("conn-a")));

        SyncResult result = client().definitions().sync(set);

        assertInstanceOf(Category.Failed.class, result.connections());
        Category.Failed subs = assertInstanceOf(Category.Failed.class, result.subscriptions());
        assertTrue(subs.error().contains("connection sync failed"), "says why: " + subs.error());
        assertTrue(callsTo("subscriptions/sync").isEmpty(), "no subscriptions request sent");
    }
}
