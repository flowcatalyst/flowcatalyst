package tech.flowcatalyst.messagerouter.endpoint;

import io.quarkus.test.InjectMock;
import io.quarkus.test.junit.QuarkusTest;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.messagerouter.health.InfrastructureHealthService;

import java.util.List;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for HealthCheckResource endpoint
 */
@QuarkusTest
class HealthCheckResourceTest {

    @InjectMock
    InfrastructureHealthService infrastructureHealthService;

    @BeforeEach
    void setUp() {
        reset(infrastructureHealthService);
    }

    @Test
    void shouldReturn200WhenInfrastructureHealthy() {
        // Given
        InfrastructureHealthService.InfrastructureHealth healthyStatus =
            new InfrastructureHealthService.InfrastructureHealth(
                true,
                "Infrastructure is operational",
                null
            );

        when(infrastructureHealthService.checkHealth()).thenReturn(healthyStatus);

        // When/Then
        given()
            .when().get("/health")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("healthy", equalTo(true))
            .body("message", equalTo("Infrastructure is operational"))
            .body("issues", nullValue());

        verify(infrastructureHealthService).checkHealth();
    }

    @Test
    void shouldReturn503WhenInfrastructureUnhealthy() {
        // Given
        InfrastructureHealthService.InfrastructureHealth unhealthyStatus =
            new InfrastructureHealthService.InfrastructureHealth(
                false,
                "Infrastructure issues detected",
                List.of("QueueManager not initialized", "No active process pools")
            );

        when(infrastructureHealthService.checkHealth()).thenReturn(unhealthyStatus);

        // When/Then
        given()
            .when().get("/health")
            .then()
            .statusCode(503)
            .contentType("application/json")
            .body("healthy", equalTo(false))
            .body("message", equalTo("Infrastructure issues detected"))
            .body("issues", hasSize(2))
            .body("issues[0]", equalTo("QueueManager not initialized"))
            .body("issues[1]", equalTo("No active process pools"));

        verify(infrastructureHealthService).checkHealth();
    }

    @Test
    void shouldReturn503WhenAllPoolsStalled() {
        // Given
        InfrastructureHealthService.InfrastructureHealth stalledStatus =
            new InfrastructureHealthService.InfrastructureHealth(
                false,
                "Infrastructure issues detected",
                List.of("All process pools appear stalled (no activity in 120s)")
            );

        when(infrastructureHealthService.checkHealth()).thenReturn(stalledStatus);

        // When/Then
        given()
            .when().get("/health")
            .then()
            .statusCode(503)
            .contentType("application/json")
            .body("healthy", equalTo(false))
            .body("issues", hasSize(1))
            .body("issues[0]", containsString("stalled"));

        verify(infrastructureHealthService).checkHealth();
    }

    @Test
    void shouldReturnCorrectJsonStructure() {
        // Given
        InfrastructureHealthService.InfrastructureHealth healthyStatus =
            new InfrastructureHealthService.InfrastructureHealth(
                true,
                "Infrastructure is operational",
                null
            );

        when(infrastructureHealthService.checkHealth()).thenReturn(healthyStatus);

        // When/Then - validate JSON structure
        given()
            .when().get("/health")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("$", hasKey("healthy"))
            .body("$", hasKey("message"))
            .body("healthy", instanceOf(Boolean.class))
            .body("message", instanceOf(String.class));
    }

    @Test
    void shouldHandleMultipleIssues() {
        // Given
        InfrastructureHealthService.InfrastructureHealth unhealthyStatus =
            new InfrastructureHealthService.InfrastructureHealth(
                false,
                "Infrastructure issues detected",
                List.of(
                    "Issue 1",
                    "Issue 2",
                    "Issue 3"
                )
            );

        when(infrastructureHealthService.checkHealth()).thenReturn(unhealthyStatus);

        // When/Then
        given()
            .when().get("/health")
            .then()
            .statusCode(503)
            .contentType("application/json")
            .body("issues", hasSize(3));

        verify(infrastructureHealthService).checkHealth();
    }

    @Test
    void shouldReturn200WhenMessageRouterDisabled() {
        // Given
        InfrastructureHealthService.InfrastructureHealth disabledStatus =
            new InfrastructureHealthService.InfrastructureHealth(
                true,
                "Message router is disabled",
                null
            );

        when(infrastructureHealthService.checkHealth()).thenReturn(disabledStatus);

        // When/Then
        given()
            .when().get("/health")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("healthy", equalTo(true))
            .body("message", equalTo("Message router is disabled"));

        verify(infrastructureHealthService).checkHealth();
    }
}
