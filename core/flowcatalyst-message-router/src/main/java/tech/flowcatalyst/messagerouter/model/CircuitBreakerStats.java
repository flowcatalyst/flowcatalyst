package tech.flowcatalyst.messagerouter.model;

import org.eclipse.microprofile.openapi.annotations.media.Schema;

/**
 * Circuit breaker statistics
 */
@Schema(description = "Circuit breaker statistics and state")
public record CircuitBreakerStats(
    @Schema(description = "Circuit breaker name/identifier", examples = {"http://localhost:8081/api/slow", "https://api.example.com/endpoint"})
    String name,

    @Schema(description = "Circuit breaker state", examples = {"CLOSED", "OPEN", "HALF_OPEN"})
    String state,

    @Schema(description = "Number of successful calls", examples = {"10000", "0", "5432"})
    long successfulCalls,

    @Schema(description = "Number of failed calls", examples = {"0", "50", "100"})
    long failedCalls,

    @Schema(description = "Number of rejected calls while circuit is open", examples = {"0", "25", "500"})
    long rejectedCalls,

    @Schema(description = "Failure rate (0.0 to 1.0)", examples = {"0.0", "0.05", "0.5"})
    double failureRate,

    @Schema(description = "Number of calls in the ring buffer", examples = {"100", "50", "0"})
    int bufferedCalls,

    @Schema(description = "Maximum ring buffer size", examples = {"100", "50", "200"})
    int bufferSize
) {}
