package tech.flowcatalyst.messagerouter.integration;

import com.github.tomakehurst.wiremock.WireMockServer;
import io.quarkus.test.common.QuarkusTestResourceLifecycleManager;

import java.util.Map;

import static com.github.tomakehurst.wiremock.client.WireMock.*;
import static com.github.tomakehurst.wiremock.core.WireMockConfiguration.options;

/**
 * Quarkus test resource that starts a WireMock server for E2E integration tests.
 *
 * <p>Provides mock HTTP endpoints for testing message mediation without requiring
 * real downstream services. The WireMock server is started once per test class
 * and provides default stubs for common scenarios:
 *
 * <ul>
 *   <li>/webhook/success - Returns 200 OK immediately</li>
 *   <li>/webhook/slow - Returns 200 OK with 5 second delay</li>
 *   <li>/webhook/error - Returns 500 Internal Server Error</li>
 *   <li>/webhook/client-error - Returns 400 Bad Request</li>
 * </ul>
 *
 * <p>Usage:
 * <pre>
 * {@code
 * @QuarkusTest
 * @QuarkusTestResource(WireMockTestResource.class)
 * class MyE2ETest {
 *
 *     @ConfigProperty(name = "test.webhook.baseurl")
 *     String webhookBaseUrl;
 *
 *     @Test
 *     void shouldCallWebhook() {
 *         // WireMock server is running at webhookBaseUrl
 *         // Send message with mediationTarget: webhookBaseUrl + "/webhook/success"
 *     }
 * }
 * }
 * </pre>
 *
 * @see <a href="https://wiremock.org/">WireMock Documentation</a>
 */
public class WireMockTestResource implements QuarkusTestResourceLifecycleManager {

    private WireMockServer wireMockServer;

    @Override
    public Map<String, String> start() {
        // Start WireMock on dynamic port
        wireMockServer = new WireMockServer(options()
            .dynamicPort()
        );
        wireMockServer.start();

        // Configure default stubs for common test scenarios
        configureDefaultStubs();

        // Make base URL available to tests
        return Map.of(
            "test.webhook.baseurl", wireMockServer.baseUrl()
        );
    }

    @Override
    public void stop() {
        if (wireMockServer != null) {
            wireMockServer.stop();
        }
    }

    /**
     * Get the WireMock server instance for advanced stubbing in tests.
     *
     * <p>Note: This is primarily for documentation. In actual tests, you would
     * inject the server via a more sophisticated mechanism or use the base URL
     * and create stubs via the WireMock client API.
     *
     * @return The WireMock server instance
     */
    public WireMockServer getServer() {
        return wireMockServer;
    }

    /**
     * Configure default stubs for common test scenarios.
     * Tests can override these or add additional stubs as needed.
     */
    private void configureDefaultStubs() {
        // Success endpoint - returns 200 OK with ack: true
        wireMockServer.stubFor(post(urlEqualTo("/webhook/success"))
            .willReturn(aResponse()
                .withStatus(200)
                .withHeader("Content-Type", "application/json")
                .withBody("{\"ack\":true,\"message\":\"\"}")));

        // Slow endpoint - returns 200 OK with ack: true after 5 second delay
        wireMockServer.stubFor(post(urlEqualTo("/webhook/slow"))
            .willReturn(aResponse()
                .withStatus(200)
                .withHeader("Content-Type", "application/json")
                .withBody("{\"ack\":true,\"message\":\"\"}")
                .withFixedDelay(5000)));

        // Server error endpoint - returns 500 Internal Server Error
        wireMockServer.stubFor(post(urlEqualTo("/webhook/error"))
            .willReturn(aResponse()
                .withStatus(500)
                .withHeader("Content-Type", "application/json")
                .withBody("{\"error\":\"Internal server error\"}")));

        // Client error endpoint - returns 400 Bad Request
        wireMockServer.stubFor(post(urlEqualTo("/webhook/client-error"))
            .willReturn(aResponse()
                .withStatus(400)
                .withHeader("Content-Type", "application/json")
                .withBody("{\"error\":\"Record not found\"}")));

        // Pending endpoint - returns 200 OK with ack: false (message not ready yet)
        wireMockServer.stubFor(post(urlEqualTo("/webhook/pending"))
            .willReturn(aResponse()
                .withStatus(200)
                .withHeader("Content-Type", "application/json")
                .withBody("{\"ack\":false,\"message\":\"notBefore time not reached\"}")));

        // Timeout endpoint - never responds (useful for testing timeouts)
        wireMockServer.stubFor(post(urlEqualTo("/webhook/timeout"))
            .willReturn(aResponse()
                .withStatus(200)
                .withFixedDelay(60000))); // 60 seconds (exceeds typical timeout)
    }
}
