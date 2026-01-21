package tech.flowcatalyst.messagerouter.traffic;

import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

@QuarkusTest
public class TrafficManagementServiceTest {

    @Inject
    TrafficManagementService trafficManagementService;

    @Test
    public void testTrafficManagementServiceAvailable() {
        assertNotNull(trafficManagementService, "TrafficManagementService should be injected");
    }

    @Test
    public void testGetStatusWithNoOpStrategy() {
        // With default config (traffic-management.enabled=false), should use NoOp strategy
        var status = trafficManagementService.getStatus();

        assertNotNull(status, "Status should not be null");
        assertEquals("noop", status.strategyType, "Should use noop strategy by default");
        assertTrue(status.registered, "NoOp strategy should always report as registered");
        assertNull(status.lastError, "NoOp strategy should have no errors");
    }

    @Test
    public void testRegisterAndDeregisterOperations() {
        // These should not throw exceptions even with NoOp strategy
        assertDoesNotThrow(() -> trafficManagementService.registerAsActive(),
                "registerAsActive should not throw");
        assertDoesNotThrow(() -> trafficManagementService.deregisterFromActive(),
                "deregisterFromActive should not throw");
    }
}
