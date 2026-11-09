package io.flowcatalyst.sdk.sync;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.push.TicketMinter;
import io.flowcatalyst.sdk.sync.Definitions.DefinitionSet;
import io.flowcatalyst.sdk.sync.Definitions.PushDefinition;
import io.flowcatalyst.sdk.sync.Definitions.PushRule;
import io.flowcatalyst.sdk.sync.SyncResult.Category;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/** Rules and ticket keys sync to {@code POST /api/applications/{app}/push/sync} (spec section 8). */
class PushSyncTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final String SEED_A = "5BgJptHbkz4uIeRqgLz3Ws_38Ovwi1uCpSdyW45yMdc";
    private static final String SEED_B = "aMAmPCO41Sx6WwDvG_MWCgxTdygpi14ynrtSTxTfFrM";
    private static final String OK = "{\"applicationCode\":\"shop\",\"keys\":{\"created\":1,\"updated\":0,\"deleted\":1},"
            + "\"rules\":{\"created\":2,\"updated\":1,\"deleted\":0},\"syncedKids\":[\"k1\"],"
            + "\"syncedPatterns\":[\"shop:orders:{principal}\",\"shop:board\"]}";

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
        return FlowCatalystClient.builder().baseUrl(server.baseUrl()).clientCredentials("id", "secret").build();
    }

    private static PushDefinition definition(String kid, String seed, PushRule... rules) {
        return PushDefinition.of(List.of(rules), TicketMinter.fromConfig("shop", kid + ":" + seed, List.of()));
    }

    private JsonNode pushBody() throws Exception {
        return MAPPER.readTree(server.requests.stream()
                .filter(r -> r.pathAndQuery().equals("/api/applications/shop/push/sync"))
                .findFirst().orElseThrow().body());
    }

    @Test
    void postsTheRulesAndTheKeysAndReportsTheGatewaysCounts() throws Exception {
        server.on("POST", "/api/applications/shop/push/sync", 200, OK);
        DefinitionSet set = DefinitionSet.define("shop").withPush(definition("k1", SEED_A,
                PushRule.of("shop:orders:{principal}"), PushRule.of("shop:admin:{id}", "shop:admin:read", "shop:admin:write"),
                PushRule.open("shop:board")));

        SyncResult result = client().definitions().sync(set);

        Category.PushSynced push = assertInstanceOf(Category.PushSynced.class, result.push());
        assertEquals(new Category.PushSynced.Counts(2, 1, 0), push.rules());
        assertEquals(new Category.PushSynced.Counts(1, 0, 1), push.keys());
        assertEquals(3, push.created());
        assertEquals(1, push.updated());
        assertEquals(1, push.deleted());
        assertEquals(List.of("k1"), push.syncedKids());
        assertEquals(List.of("shop:orders:{principal}", "shop:board"), push.syncedPatterns());
        assertEquals(Category.SKIPPED, result.roles());

        JsonNode body = pushBody();
        assertEquals(List.of("rules", "keys"), body.propertyNames().stream().toList());
        JsonNode rules = body.get("rules");
        assertEquals("{\"pattern\":\"shop:orders:{principal}\"}", rules.get(0).toString(), "no empty requires, no public:false");
        assertEquals("{\"pattern\":\"shop:admin:{id}\",\"requires\":[\"shop:admin:read\",\"shop:admin:write\"]}", rules.get(1).toString());
        assertEquals("{\"pattern\":\"shop:board\",\"public\":true}", rules.get(2).toString());
        JsonNode key = body.get("keys").get("keys").get(0);
        assertEquals("k1", key.get("kid").asString());
        assertEquals("OKP", key.get("kty").asString());
        assertEquals("Ed25519", key.get("crv").asString());
        assertEquals("C5tnrYkj67IDkzahPXBR70jrqnjByoUmQMvv4oOk5S8", key.get("x").asString());
        assertTrue(!key.has("d"), "no private member");
    }

    @Test
    void aSetWithoutPushSendsNothingAndSkipPushForcesIt() {
        server.on("POST", "/api/applications/shop/push/sync", 200, OK);
        SyncResult none = client().definitions().sync(DefinitionSet.define("shop"));
        assertEquals(Category.SKIPPED, none.push());

        DefinitionSet set = DefinitionSet.define("shop").withPush(definition("k1", SEED_A, PushRule.open("shop:board")));
        SyncResult skipped = client().definitions().sync(set,
                SyncOptions.defaults().skipping(SyncOptions.SyncCategory.PUSH));
        assertEquals(Category.SKIPPED, skipped.push());
        assertTrue(server.requests.stream().noneMatch(r -> r.pathAndQuery().contains("/push/")));
    }

    @Test
    void theGatewaysRefusalIsAFailedCategoryAndTheSyncThrows() {
        server.on("POST", "/api/applications/shop/push/sync", 400,
                "{\"code\":\"RULE_PATTERN_FOREIGN\",\"message\":\"the pattern is another application's\"}");
        DefinitionSet set = DefinitionSet.define("shop").withPush(definition("k1", SEED_A, PushRule.open("other:board")));

        DefinitionSyncException e = assertThrows(DefinitionSyncException.class, () -> client().definitions().sync(set));
        assertInstanceOf(Category.Failed.class, e.result().push());
        assertTrue(e.getMessage().contains("push"), e.getMessage());
    }

    @Test
    void syncPushIsUsableOnItsOwn() {
        server.on("POST", "/api/applications/shop/push/sync", 200, OK);
        Category result = client().definitions().syncPush("shop", definition("k1", SEED_A, PushRule.open("shop:board")));
        assertInstanceOf(Category.PushSynced.class, result);
    }

    @Test
    void setsOfOneApplicationMergeIntoOneDeclarationWithKeysUnionedByKid() throws Exception {
        server.on("POST", "/api/applications/shop/push/sync", 200, OK);
        DefinitionSet a = DefinitionSet.define("shop").withPush(definition("k1", SEED_A, PushRule.open("shop:board")));
        DefinitionSet b = DefinitionSet.define("shop").withPush(PushDefinition.of(List.of(PushRule.of("shop:orders:{principal}")),
                new TicketMinter("shop", io.flowcatalyst.sdk.push.TicketKey.parse("k1:" + SEED_A),
                        List.of(io.flowcatalyst.sdk.push.TicketKey.parse("k0:" + SEED_B).toPublicJwk()))));

        client().definitions().syncGrouped(List.of(a, b));

        assertEquals(1, server.requests.stream().filter(r -> r.pathAndQuery().contains("/push/sync")).count(), "one call");
        JsonNode body = pushBody();
        assertEquals(2, body.get("rules").size());
        assertEquals(List.of("k1", "k0"), body.get("keys").get("keys").valueStream().map(k -> k.get("kid").asString()).toList());
    }

    @Test
    void theElevenArgumentSyncResultStillBuilds() {
        SyncResult r = new SyncResult("shop", Category.SKIPPED, Category.SKIPPED, Category.SKIPPED, Category.SKIPPED,
                Category.SKIPPED, Category.SKIPPED, Category.SKIPPED, Category.SKIPPED, Category.SKIPPED, Category.SKIPPED);
        assertEquals(Category.SKIPPED, r.push());
    }
}
