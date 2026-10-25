package io.flowcatalyst.sdk.resources;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertInstanceOf;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import io.flowcatalyst.sdk.FlowCatalystClient;
import io.flowcatalyst.sdk.StubServer;
import io.flowcatalyst.sdk.error.FlowCatalystException;
import io.flowcatalyst.sdk.error.SdkError;
import java.time.Duration;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** Platform-config reads and the dedicated, audited SECRET reveal call. */
class PlatformConfigResourceTest {

    private static final String PLAINTEXT = "s3cr3t-plaintext-value";

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

    private String lastPath() {
        return server.requests.getLast().pathAndQuery();
    }

    private static String secretBody() {
        return "{\"applicationCode\":\"orders\",\"section\":\"smtp\",\"property\":\"password\","
                + "\"value\":\"" + PLAINTEXT + "\",\"updatedAt\":\"2026-10-22T10:00:00Z\"}";
    }

    @Test
    void revealSendsGetToSecretPathAndOmitsClientIdWhenAbsent() {
        server.on("GET", "/api/config/orders/smtp/password/secret", 200, secretBody());

        var secret = client().platformConfig().revealSecret("orders", "smtp", "password", null);

        assertEquals("GET", server.requests.getLast().method());
        assertEquals("/api/config/orders/smtp/password/secret", lastPath());
        assertEquals(PLAINTEXT, secret.value());
        assertEquals("orders", secret.applicationCode());
        assertEquals("smtp", secret.section());
        assertEquals("password", secret.property());
        assertEquals(2026, secret.updatedAt().getYear());
    }

    @Test
    void revealEncodesSegmentsAndSendsClientIdWhenGiven() {
        server.on("GET", "/api/config/a b/s/x/p?q/secret", 200, secretBody());

        client().platformConfig().revealSecret("a b", "s/x", "p?q", "clt_1");

        // The stub records the server-decoded path: "s/x" and "p?q" stayed single segments (no extra
        // path split, no query injection) because they were encoded on the wire.
        assertEquals("/api/config/a b/s/x/p?q/secret?clientId=clt_1", lastPath());
    }

    @Test
    void revealErrorsMapToTheNormalErrorTypes() {
        server.on("GET", "/api/config/a/s/forbidden/secret", 403, "{\"error\":\"FORBIDDEN\",\"message\":\"no\"}");
        server.on("GET", "/api/config/a/s/missing/secret", 404, "{\"error\":\"NOT_FOUND\",\"message\":\"none\"}");
        server.on("GET", "/api/config/a/s/plain/secret", 400,
                "{\"error\":\"NOT_A_SECRET\",\"message\":\"property is PLAIN\"}");

        var cfg = client().platformConfig();
        assertInstanceOf(SdkError.Forbidden.class,
                assertThrows(FlowCatalystException.class, () -> cfg.revealSecret("a", "s", "forbidden", null)).error());
        assertInstanceOf(SdkError.NotFound.class,
                assertThrows(FlowCatalystException.class, () -> cfg.revealSecret("a", "s", "missing", null)).error());
        var notSecret = assertInstanceOf(SdkError.HttpStatus.class,
                assertThrows(FlowCatalystException.class, () -> cfg.revealSecret("a", "s", "plain", null)).error());
        assertEquals(400, notSecret.status());
        assertTrue(notSecret.body().contains("NOT_A_SECRET"));
    }

    @Test
    void revealedSecretToStringDoesNotContainTheValue() {
        server.on("GET", "/api/config/orders/smtp/password/secret", 200, secretBody());

        var secret = client().platformConfig().revealSecret("orders", "smtp", "password", null);

        assertFalse(secret.toString().contains(PLAINTEXT), secret.toString());
        assertTrue(secret.toString().contains("***"));
    }

    @Test
    void getAndListSendClientIdOnlyWhenGivenAndReturnMaskedSecrets() {
        String item = "{\"id\":\"c1\",\"applicationCode\":\"orders\",\"section\":\"smtp\",\"property\":\"password\","
                + "\"scope\":\"GLOBAL\",\"value\":\"***\",\"valueType\":\"SECRET\"}";
        server.on("GET", "/api/config/orders/smtp/password", 200, item);
        server.on("GET", "/api/platform-config/orders", 200, "{\"items\":[" + item + "]}");

        var cfg = client().platformConfig();
        assertEquals("***", cfg.get("orders", "smtp", "password", null).getValue());
        assertEquals("/api/config/orders/smtp/password", lastPath());
        cfg.get("orders", "smtp", "password", "clt_1");
        assertEquals("/api/config/orders/smtp/password?clientId=clt_1", lastPath());
        assertEquals(1, cfg.list("orders").getItems().size());
    }

    @Test
    void listRowsCarryTheClientIdentifier() {
        String item = "{\"id\":\"c1\",\"applicationCode\":\"orders\",\"section\":\"smtp\",\"property\":\"host\","
                + "\"scope\":\"CLIENT\",\"clientId\":\"clt_1\",\"clientIdentifier\":\"acme\","
                + "\"value\":\"h\",\"valueType\":\"PLAIN\"}";
        server.on("GET", "/api/platform-config/orders", 200, "{\"items\":[" + item + "]}");

        var row = client().platformConfig().list("orders").getItems().getFirst();
        assertEquals("clt_1", row.getClientId());
        assertEquals("acme", row.getClientIdentifier());
    }

    @Test
    void setDeleteAndRevealSendOnBehalfOfOnlyWhenGiven() {
        String item = "{\"id\":\"c1\",\"applicationCode\":\"orders\",\"section\":\"smtp\",\"property\":\"password\","
                + "\"scope\":\"GLOBAL\",\"value\":\"***\",\"valueType\":\"SECRET\"}";
        server.on("PUT", "/api/config/orders/smtp/password", 200, item);
        server.on("DELETE", "/api/config/orders/smtp/password", 204, null);
        server.on("GET", "/api/config/orders/smtp/password/secret", 200, secretBody());

        var cfg = client().platformConfig();
        var req = new io.flowcatalyst.sdk.generated.model.SetPropertyRequest().value("v").valueType("SECRET");
        cfg.set("orders", "smtp", "password", req, "usr:alice@example.com");
        assertEquals("PUT", server.requests.getLast().method());
        assertEquals("usr:alice@example.com", server.requests.getLast().onBehalfOf());
        cfg.set("orders", "smtp", "password", req);
        assertEquals(null, server.requests.getLast().onBehalfOf());

        cfg.delete("orders", "smtp", "password", "clt_1", "bob");
        assertEquals("DELETE", server.requests.getLast().method());
        assertEquals("/api/config/orders/smtp/password?clientId=clt_1", lastPath());
        assertEquals("bob", server.requests.getLast().onBehalfOf());
        cfg.delete("orders", "smtp", "password", null, "  ");
        assertEquals("/api/config/orders/smtp/password", lastPath());
        assertEquals(null, server.requests.getLast().onBehalfOf());

        cfg.revealSecret("orders", "smtp", "password", null, "carol");
        assertEquals("carol", server.requests.getLast().onBehalfOf());
        assertEquals(PlatformConfigResource.ON_BEHALF_OF, "X-On-Behalf-Of");
    }
}
