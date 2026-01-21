package tech.flowcatalyst.messagerouter.metrics;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.search.Search;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.model.CircuitBreakerStats;

import java.util.HashMap;
import java.util.Map;

@ApplicationScoped
public class CircuitBreakerMetricsService {

    private static final Logger LOG = Logger.getLogger(CircuitBreakerMetricsService.class);

    @Inject
    MeterRegistry meterRegistry;

    /**
     * Get all circuit breaker statistics from Micrometer metrics
     */
    public Map<String, CircuitBreakerStats> getAllCircuitBreakerStats() {
        Map<String, CircuitBreakerStats> allStats = new HashMap<>();

        try {
            // SmallRye Fault Tolerance registers metrics with prefix "ft."
            // Look for circuit breaker metrics in the registry
            Search.in(meterRegistry)
                .name(name -> name.startsWith("ft.") && name.contains("circuitbreaker"))
                .meters()
                .forEach(meter -> {
                    String meterName = meter.getId().getName();

                    // Extract circuit breaker name from metric tags
                    String cbName = meter.getId().getTag("method");
                    if (cbName != null && !allStats.containsKey(cbName)) {
                        // Get basic stats from available metrics
                        long successCalls = getCounterValue("ft.invocations.total", cbName, "valueReturned");
                        long failedCalls = getCounterValue("ft.invocations.total", cbName, "exceptionThrown");
                        long rejectedCalls = getCounterValue("ft.circuitbreaker.calls.total", cbName, "circuitBreakerOpen");

                        String state = getCircuitBreakerStateFromMetrics(cbName);

                        CircuitBreakerStats stats = new CircuitBreakerStats(
                            cbName,
                            state,
                            successCalls,
                            failedCalls,
                            rejectedCalls,
                            calculateFailureRate(successCalls, failedCalls),
                            0, // Not easily available
                            0  // Not easily available
                        );

                        allStats.put(cbName, stats);
                    }
                });
        } catch (Exception e) {
            LOG.errorf(e, "Failed to get circuit breaker stats");
        }

        return allStats;
    }

    /**
     * Get circuit breaker state by name
     */
    public String getCircuitBreakerState(String name) {
        return getCircuitBreakerStateFromMetrics(name);
    }

    /**
     * Reset a circuit breaker - Note: SmallRye doesn't expose per-CB reset via API
     */
    public boolean resetCircuitBreaker(String name) {
        LOG.warnf("Circuit breaker reset not available via API for: %s", name);
        return false;
    }

    /**
     * Reset all circuit breakers - Note: Not available via Micrometer
     */
    public void resetAllCircuitBreakers() {
        LOG.warn("Circuit breaker reset not available via Micrometer API");
    }

    private long getCounterValue(String metricName, String methodTag, String resultTag) {
        try {
            Counter counter = Search.in(meterRegistry)
                .name(metricName)
                .tag("method", methodTag)
                .tag("result", resultTag)
                .counter();

            return counter != null ? (long) counter.count() : 0L;
        } catch (Exception e) {
            return 0L;
        }
    }

    private String getCircuitBreakerStateFromMetrics(String methodName) {
        try {
            // Check if circuit breaker is open (rejecting calls)
            long openCalls = getCounterValue("ft.circuitbreaker.calls.total", methodName, "circuitBreakerOpen");
            if (openCalls > 0) {
                return "OPEN";
            }

            // Otherwise assume closed
            return "CLOSED";
        } catch (Exception e) {
            return "UNKNOWN";
        }
    }

    private double calculateFailureRate(long successCalls, long failedCalls) {
        long total = successCalls + failedCalls;
        if (total == 0) {
            return 0.0;
        }
        return (double) failedCalls / total * 100.0;
    }
}
