package tech.flowcatalyst.messagerouter.endpoint;

import io.quarkus.test.InjectMock;
import io.quarkus.test.junit.QuarkusTest;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.messagerouter.health.HealthStatusService;
import tech.flowcatalyst.messagerouter.metrics.CircuitBreakerMetricsService;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.metrics.QueueStats;
import tech.flowcatalyst.messagerouter.model.*;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.time.Instant;
import java.util.List;
import java.util.Map;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.*;
import static org.mockito.Mockito.*;

/**
 * Comprehensive MonitoringResource endpoint tests covering:
 * - Health status endpoint
 * - Queue statistics endpoint
 * - Pool statistics endpoint
 * - Warning management endpoints
 * - Circuit breaker endpoints
 */
@QuarkusTest
class MonitoringResourceTest {

    @InjectMock
    QueueMetricsService queueMetricsService;

    @InjectMock
    PoolMetricsService poolMetricsService;

    @InjectMock
    WarningService warningService;

    @InjectMock
    CircuitBreakerMetricsService circuitBreakerMetricsService;

    @InjectMock
    HealthStatusService healthStatusService;

    @BeforeEach
    void setUp() {
        reset(queueMetricsService, poolMetricsService, warningService, circuitBreakerMetricsService, healthStatusService);
    }

    @Test
    void shouldGetHealthStatus() {
        // Given
        HealthStatus.HealthDetails details = new HealthStatus.HealthDetails(
            2,  // totalQueues
            2,  // healthyQueues
            1,  // totalPools
            1,  // healthyPools
            0,  // activeWarnings
            0,  // criticalWarnings
            0,  // circuitBreakersOpen
            null // degradationReason
        );

        HealthStatus mockHealth = new HealthStatus(
            "HEALTHY",
            Instant.now(),
            100000L,
            details
        );

        when(healthStatusService.getHealthStatus()).thenReturn(mockHealth);

        // When/Then
        given()
            .when().get("/monitoring/health")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("status", equalTo("HEALTHY"))
            .body("details.totalQueues", equalTo(2))
            .body("details.healthyQueues", equalTo(2));

        verify(healthStatusService).getHealthStatus();
    }

    @Test
    void shouldGetQueueStats() {
        // Given
        QueueStats queue1Stats = new QueueStats(
            "queue-1",
            100,
            80,
            20,
            0.8,
            10,
            1.5,
            50,
            15,
            100,  // totalMessages5min
            80,   // totalConsumed5min
            20,   // totalFailed5min
            0.8,  // successRate5min
            100,  // totalMessages30min
            80,   // totalConsumed30min
            20,   // totalFailed30min
            0.8,  // successRate30min
            0     // totalDeferred
        );

        when(queueMetricsService.getAllQueueStats())
            .thenReturn(Map.of("queue-1", queue1Stats));

        // When/Then
        given()
            .when().get("/monitoring/queue-stats")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("'queue-1'.name", equalTo("queue-1"))
            .body("'queue-1'.totalMessages", equalTo(100))
            .body("'queue-1'.totalConsumed", equalTo(80))
            .body("'queue-1'.totalFailed", equalTo(20))
            .body("'queue-1'.pendingMessages", equalTo(50))
            .body("'queue-1'.messagesNotVisible", equalTo(15));

        verify(queueMetricsService).getAllQueueStats();
    }

    @Test
    void shouldGetPoolStats() {
        // Given
        PoolStats pool1Stats = new PoolStats(
            "POOL-A",       // poolCode
            100L,           // totalProcessed
            90L,            // totalSucceeded
            10L,            // totalFailed
            5L,             // totalRateLimited
            0.9,            // successRate
            3,              // activeWorkers
            2,              // availablePermits
            5,              // maxConcurrency
            10,             // queueSize
            100,            // maxQueueCapacity
            150.0,          // averageProcessingTimeMs
            100L,           // totalProcessed5min
            90L,            // totalSucceeded5min
            10L,            // totalFailed5min
            0.9,            // successRate5min
            100L,           // totalProcessed30min
            90L,            // totalSucceeded30min
            10L,            // totalFailed30min
            0.9,            // successRate30min
            2L,             // totalRateLimited5min
            5L              // totalRateLimited30min
        );

        when(poolMetricsService.getAllPoolStats())
            .thenReturn(Map.of("POOL-A", pool1Stats));

        // When/Then
        given()
            .when().get("/monitoring/pool-stats")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("'POOL-A'.poolCode", equalTo("POOL-A"))
            .body("'POOL-A'.activeWorkers", equalTo(3))
            .body("'POOL-A'.maxConcurrency", equalTo(5))
            .body("'POOL-A'.queueSize", equalTo(10));

        verify(poolMetricsService).getAllPoolStats();
    }

    @Test
    void shouldGetAllWarnings() {
        // Given
        Warning warning1 = new Warning(
            "warn-1",
            "QUEUE_FULL",
            "ERROR",
            "Queue is full",
            Instant.now(),
            "QueueManager",
            false
        );

        when(warningService.getAllWarnings()).thenReturn(List.of(warning1));

        // When/Then
        given()
            .when().get("/monitoring/warnings")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("size()", equalTo(1))
            .body("[0].id", equalTo("warn-1"))
            .body("[0].category", equalTo("QUEUE_FULL"))
            .body("[0].severity", equalTo("ERROR"))
            .body("[0].message", equalTo("Queue is full"));

        verify(warningService).getAllWarnings();
    }

    @Test
    void shouldGetUnacknowledgedWarnings() {
        // Given
        Warning warning1 = new Warning(
            "warn-1",
            "ROUTING",
            "WARN",
            "Routing issue",
            Instant.now(),
            "QueueManager",
            false
        );

        when(warningService.getUnacknowledgedWarnings()).thenReturn(List.of(warning1));

        // When/Then
        given()
            .when().get("/monitoring/warnings/unacknowledged")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("size()", equalTo(1))
            .body("[0].acknowledged", equalTo(false));

        verify(warningService).getUnacknowledgedWarnings();
    }

    @Test
    void shouldGetWarningsBySeverity() {
        // Given
        Warning errorWarning = new Warning(
            "warn-error",
            "ROUTING",
            "ERROR",
            "Critical error",
            Instant.now(),
            "QueueManager",
            false
        );

        when(warningService.getWarningsBySeverity("ERROR")).thenReturn(List.of(errorWarning));

        // When/Then
        given()
            .when().get("/monitoring/warnings/severity/ERROR")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("size()", equalTo(1))
            .body("[0].severity", equalTo("ERROR"));

        verify(warningService).getWarningsBySeverity("ERROR");
    }

    @Test
    void shouldAcknowledgeWarning() {
        // Given
        when(warningService.acknowledgeWarning("warn-1")).thenReturn(true);

        // When/Then
        given()
            .when().post("/monitoring/warnings/warn-1/acknowledge")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("status", equalTo("success"));

        verify(warningService).acknowledgeWarning("warn-1");
    }

    @Test
    void shouldReturn404WhenAcknowledgingNonExistentWarning() {
        // Given
        when(warningService.acknowledgeWarning("non-existent")).thenReturn(false);

        // When/Then
        given()
            .when().post("/monitoring/warnings/non-existent/acknowledge")
            .then()
            .statusCode(404)
            .contentType("application/json")
            .body("status", equalTo("error"))
            .body("message", equalTo("Warning not found"));

        verify(warningService).acknowledgeWarning("non-existent");
    }

    @Test
    void shouldClearAllWarnings() {
        // Given
        doNothing().when(warningService).clearAllWarnings();

        // When/Then
        given()
            .when().delete("/monitoring/warnings")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("status", equalTo("success"));

        verify(warningService).clearAllWarnings();
    }

    @Test
    void shouldClearOldWarnings() {
        // Given
        doNothing().when(warningService).clearOldWarnings(24);

        // When/Then
        given()
            .queryParam("hours", 24)
            .when().delete("/monitoring/warnings/old")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("status", equalTo("success"));

        verify(warningService).clearOldWarnings(24);
    }

    @Test
    void shouldClearOldWarningsWithDefaultHours() {
        // Given
        doNothing().when(warningService).clearOldWarnings(24);

        // When/Then - without specifying hours parameter
        given()
            .when().delete("/monitoring/warnings/old")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("status", equalTo("success"));

        verify(warningService).clearOldWarnings(24);
    }

    @Test
    void shouldGetCircuitBreakerStats() {
        // Given
        CircuitBreakerStats cbStats = new CircuitBreakerStats(
            "http-mediator",
            "CLOSED",
            80L,           // successfulCalls
            20L,           // failedCalls
            5L,            // rejectedCalls
            0.2,           // failureRate
            10,            // bufferedCalls
            100            // bufferSize
        );

        when(circuitBreakerMetricsService.getAllCircuitBreakerStats())
            .thenReturn(Map.of("http-mediator", cbStats));

        // When/Then
        given()
            .when().get("/monitoring/circuit-breakers")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("'http-mediator'.name", equalTo("http-mediator"))
            .body("'http-mediator'.state", equalTo("CLOSED"))
            .body("'http-mediator'.successfulCalls", equalTo(80))
            .body("'http-mediator'.failedCalls", equalTo(20));

        verify(circuitBreakerMetricsService).getAllCircuitBreakerStats();
    }

    @Test
    void shouldGetCircuitBreakerState() {
        // Given
        when(circuitBreakerMetricsService.getCircuitBreakerState("http-mediator"))
            .thenReturn("CLOSED");

        // When/Then
        given()
            .when().get("/monitoring/circuit-breakers/http-mediator/state")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("name", equalTo("http-mediator"))
            .body("state", equalTo("CLOSED"));

        verify(circuitBreakerMetricsService).getCircuitBreakerState("http-mediator");
    }

    @Test
    void shouldResetCircuitBreaker() {
        // Given
        when(circuitBreakerMetricsService.resetCircuitBreaker("http-mediator"))
            .thenReturn(true);

        // When/Then
        given()
            .when().post("/monitoring/circuit-breakers/http-mediator/reset")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("status", equalTo("success"));

        verify(circuitBreakerMetricsService).resetCircuitBreaker("http-mediator");
    }

    @Test
    void shouldReturn500WhenResetCircuitBreakerFails() {
        // Given
        when(circuitBreakerMetricsService.resetCircuitBreaker("http-mediator"))
            .thenReturn(false);

        // When/Then
        given()
            .when().post("/monitoring/circuit-breakers/http-mediator/reset")
            .then()
            .statusCode(500)
            .contentType("application/json")
            .body("status", equalTo("error"))
            .body("message", equalTo("Failed to reset circuit breaker"));

        verify(circuitBreakerMetricsService).resetCircuitBreaker("http-mediator");
    }

    @Test
    void shouldResetAllCircuitBreakers() {
        // Given
        doNothing().when(circuitBreakerMetricsService).resetAllCircuitBreakers();

        // When/Then
        given()
            .when().post("/monitoring/circuit-breakers/reset-all")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("status", equalTo("success"));

        verify(circuitBreakerMetricsService).resetAllCircuitBreakers();
    }

    @Test
    void shouldHandleMultipleQueueStats() {
        // Given
        QueueStats queue1 = new QueueStats("queue-1", 100, 80, 20, 0.8, 10, 1.5, 50, 15, 100, 80, 20, 0.8, 100, 80, 20, 0.8, 0);
        QueueStats queue2 = new QueueStats("queue-2", 200, 180, 20, 0.9, 5, 2.0, 75, 20, 200, 180, 20, 0.9, 200, 180, 20, 0.9, 0);

        when(queueMetricsService.getAllQueueStats())
            .thenReturn(Map.of("queue-1", queue1, "queue-2", queue2));

        // When/Then
        given()
            .when().get("/monitoring/queue-stats")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("size()", equalTo(2))
            .body("containsKey('queue-1')", equalTo(true))
            .body("containsKey('queue-2')", equalTo(true));

        verify(queueMetricsService).getAllQueueStats();
    }

    @Test
    void shouldHandleEmptyWarningsList() {
        // Given
        when(warningService.getAllWarnings()).thenReturn(List.of());

        // When/Then
        given()
            .when().get("/monitoring/warnings")
            .then()
            .statusCode(200)
            .contentType("application/json")
            .body("size()", equalTo(0));

        verify(warningService).getAllWarnings();
    }
}
