package tech.flowcatalyst.messagerouter.endpoint;

import io.quarkus.test.InjectMock;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import jakarta.ws.rs.core.Response;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.messagerouter.health.BrokerHealthService;
import tech.flowcatalyst.messagerouter.health.InfrastructureHealthService;
import tech.flowcatalyst.messagerouter.health.InfrastructureHealthService.InfrastructureHealth;
import tech.flowcatalyst.messagerouter.model.ReadinessStatus;

import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Tests for Kubernetes-style health endpoints covering:
 * - Liveness probe (should always pass if application is running)
 * - Readiness probe (checks infrastructure and broker)
 * - Startup probe
 * - Proper HTTP status codes
 */
@QuarkusTest
class KubernetesHealthResourceTest {

    @Inject
    KubernetesHealthResource healthResource;

    @InjectMock
    InfrastructureHealthService infrastructureHealthService;

    @InjectMock
    BrokerHealthService brokerHealthService;

    @Test
    void shouldPassLivenessCheck() {
        // When
        Response response = healthResource.liveness();

        // Then
        assertEquals(200, response.getStatus(), "Liveness should return 200");

        ReadinessStatus status = (ReadinessStatus) response.getEntity();
        assertEquals("ALIVE", status.status());
        assertTrue(status.issues().isEmpty());
    }

    @Test
    void shouldPassReadinessCheckWhenAllHealthy() {
        // Given
        when(infrastructureHealthService.checkHealth())
            .thenReturn(new InfrastructureHealth(true, "All systems operational", List.of()));
        when(brokerHealthService.checkBrokerConnectivity())
            .thenReturn(List.of());

        // When
        Response response = healthResource.readiness();

        // Then
        assertEquals(200, response.getStatus(), "Readiness should return 200 when healthy");

        ReadinessStatus status = (ReadinessStatus) response.getEntity();
        assertEquals("READY", status.status());
        assertTrue(status.issues().isEmpty());
    }

    @Test
    void shouldFailReadinessCheckWhenInfrastructureUnhealthy() {
        // Given
        when(infrastructureHealthService.checkHealth())
            .thenReturn(new InfrastructureHealth(
                false,
                "Infrastructure unhealthy",
                List.of("QueueManager not initialized")
            ));
        when(brokerHealthService.checkBrokerConnectivity())
            .thenReturn(List.of());

        // When
        Response response = healthResource.readiness();

        // Then
        assertEquals(503, response.getStatus(), "Readiness should return 503 when infrastructure unhealthy");

        ReadinessStatus status = (ReadinessStatus) response.getEntity();
        assertEquals("NOT_READY", status.status());
        assertTrue(status.issues().contains("QueueManager not initialized"));
    }

    @Test
    void shouldFailReadinessCheckWhenBrokerUnreachable() {
        // Given
        when(infrastructureHealthService.checkHealth())
            .thenReturn(new InfrastructureHealth(true, "Infrastructure healthy", List.of()));
        when(brokerHealthService.checkBrokerConnectivity())
            .thenReturn(List.of("SQS broker is not accessible"));

        // When
        Response response = healthResource.readiness();

        // Then
        assertEquals(503, response.getStatus(), "Readiness should return 503 when broker unreachable");

        ReadinessStatus status = (ReadinessStatus) response.getEntity();
        assertEquals("NOT_READY", status.status());
        assertTrue(status.issues().contains("SQS broker is not accessible"));
    }

    @Test
    void shouldFailReadinessCheckWithMultipleIssues() {
        // Given
        when(infrastructureHealthService.checkHealth())
            .thenReturn(new InfrastructureHealth(
                false,
                "Infrastructure unhealthy",
                List.of("All pools stalled")
            ));
        when(brokerHealthService.checkBrokerConnectivity())
            .thenReturn(List.of("ActiveMQ broker connectivity check failed"));

        // When
        Response response = healthResource.readiness();

        // Then
        assertEquals(503, response.getStatus(), "Readiness should return 503 with multiple issues");

        ReadinessStatus status = (ReadinessStatus) response.getEntity();
        assertEquals("NOT_READY", status.status());
        assertEquals(2, status.issues().size());
        assertTrue(status.issues().contains("All pools stalled"));
        assertTrue(status.issues().contains("ActiveMQ broker connectivity check failed"));
    }

    @Test
    void shouldHandleExceptionInReadinessCheck() {
        // Given
        when(infrastructureHealthService.checkHealth())
            .thenThrow(new RuntimeException("Unexpected error"));

        // When
        Response response = healthResource.readiness();

        // Then
        assertEquals(503, response.getStatus(), "Readiness should return 503 on exception");

        ReadinessStatus status = (ReadinessStatus) response.getEntity();
        assertEquals("ERROR", status.status());
        assertFalse(status.issues().isEmpty());
    }

    @Test
    void shouldReturnStartupStatus() {
        // Given - startup uses same logic as readiness
        when(infrastructureHealthService.checkHealth())
            .thenReturn(new InfrastructureHealth(true, "All systems operational", List.of()));
        when(brokerHealthService.checkBrokerConnectivity())
            .thenReturn(List.of());

        // When
        Response response = healthResource.startup();

        // Then
        assertEquals(200, response.getStatus(), "Startup should return 200 when ready");
    }

    @Test
    void shouldIncludeTimestampInHealthStatus() {
        // When
        Response response = healthResource.liveness();

        // Then
        ReadinessStatus status = (ReadinessStatus) response.getEntity();
        assertNotNull(status.timestamp());
    }

    @Test
    void shouldVerifyInfrastructureAndBrokerChecksAreCalled() {
        // Given
        when(infrastructureHealthService.checkHealth())
            .thenReturn(new InfrastructureHealth(true, "All systems operational", List.of()));
        when(brokerHealthService.checkBrokerConnectivity())
            .thenReturn(List.of());

        // When
        healthResource.readiness();

        // Then
        verify(infrastructureHealthService, times(1)).checkHealth();
        verify(brokerHealthService, times(1)).checkBrokerConnectivity();
    }
}
