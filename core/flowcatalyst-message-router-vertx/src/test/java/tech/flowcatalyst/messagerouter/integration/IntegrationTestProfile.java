package tech.flowcatalyst.messagerouter.integration;

import io.quarkus.test.junit.QuarkusTestProfile;

import java.util.Map;

/**
 * Test profile for integration tests that need the message router enabled.
 */
public class IntegrationTestProfile implements QuarkusTestProfile {

    @Override
    public Map<String, String> getConfigOverrides() {
        return Map.of(
            "message-router.enabled", "true"
        );
    }
}
