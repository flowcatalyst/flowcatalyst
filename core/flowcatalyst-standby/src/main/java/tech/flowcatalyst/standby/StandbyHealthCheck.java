package tech.flowcatalyst.standby;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Instance;
import jakarta.inject.Inject;
import org.eclipse.microprofile.health.HealthCheck;
import org.eclipse.microprofile.health.HealthCheckResponse;
import org.eclipse.microprofile.health.Liveness;
import org.eclipse.microprofile.health.Readiness;

/**
 * Health check for standby mode.
 * Reports DOWN if standby is enabled but Redis is unavailable.
 * This ensures:
 * - Liveness probe fails (K8s will restart the pod)
 * - Readiness probe fails (K8s will remove from load balancer)
 * - System appears unhealthy until Redis is restored
 */
@ApplicationScoped
@Liveness
@Readiness
public class StandbyHealthCheck implements HealthCheck {

    @Inject
    StandbyConfig standbyConfig;

    @Inject
    Instance<StandbyService> standbyServiceInstance;

    @Override
    public HealthCheckResponse call() {
        // If standby is not enabled, don't affect health
        if (!standbyConfig.enabled()) {
            return HealthCheckResponse.builder().name("Standby").up().build();
        }

        // If service not injected, standby is disabled
        if (!standbyServiceInstance.isResolvable()) {
            return HealthCheckResponse.builder().name("Standby").up().build();
        }

        StandbyService standbyService = standbyServiceInstance.get();

        // Standby is enabled - check if Redis is available
        if (!standbyService.isRedisAvailable()) {
            return HealthCheckResponse.builder()
                    .name("Standby")
                    .down()
                    .withData("reason", "Redis unavailable - standby mode broken")
                    .withData("instance", standbyConfig.instanceId())
                    .withData("message", "Redis connection failed. " +
                            "System cannot function in standby mode without Redis. " +
                            "Manual intervention required.")
                    .build();
        }

        // Redis is available - report status
        StandbyService.StandbyStatus status = standbyService.getStatus();
        return HealthCheckResponse.builder()
                .name("Standby")
                .up()
                .withData("instance", status.instanceId)
                .withData("role", status.isPrimary ? "PRIMARY" : "STANDBY")
                .withData("currentLockHolder", status.currentLockHolder)
                .withData("lastRefresh", status.lastSuccessfulRefresh != null ?
                        status.lastSuccessfulRefresh.toString() : "never")
                .build();
    }
}
