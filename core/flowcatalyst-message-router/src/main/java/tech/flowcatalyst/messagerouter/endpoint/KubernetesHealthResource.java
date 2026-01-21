package tech.flowcatalyst.messagerouter.endpoint;

import jakarta.inject.Inject;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.health.BrokerHealthService;
import tech.flowcatalyst.messagerouter.health.InfrastructureHealthService;
import tech.flowcatalyst.messagerouter.health.InfrastructureHealthService.InfrastructureHealth;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.model.ReadinessStatus;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Kubernetes-style health check endpoints following the standard probe patterns.
 *
 * <p><b>Liveness Probe</b> (/health/live):
 * <ul>
 *   <li>Checks if the application is running and not deadlocked</li>
 *   <li>Failure triggers container restart by Kubernetes</li>
 *   <li>Should NOT include external dependency checks</li>
 *   <li>Fast response (should complete in <100ms)</li>
 * </ul>
 *
 * <p><b>Readiness Probe</b> (/health/ready):
 * <ul>
 *   <li>Checks if application is ready to serve traffic</li>
 *   <li>Failure removes pod from load balancer rotation</li>
 *   <li>SHOULD include external dependency checks (broker connectivity)</li>
 *   <li>Can be slower (timeout typically 1-5 seconds)</li>
 * </ul>
 *
 * <p><b>Kubernetes Configuration Example:</b>
 * <pre>{@code
 * apiVersion: v1
 * kind: Pod
 * spec:
 *   containers:
 *   - name: flowcatalyst
 *     livenessProbe:
 *       httpGet:
 *         path: /health/live
 *         port: 8080
 *       initialDelaySeconds: 30
 *       periodSeconds: 10
 *       timeoutSeconds: 3
 *       failureThreshold: 3
 *
 *     readinessProbe:
 *       httpGet:
 *         path: /health/ready
 *         port: 8080
 *       initialDelaySeconds: 10
 *       periodSeconds: 5
 *       timeoutSeconds: 5
 *       failureThreshold: 3
 * }</pre>
 */
@Path("/health")
public class KubernetesHealthResource {

    private static final Logger LOG = Logger.getLogger(KubernetesHealthResource.class);

    @Inject
    InfrastructureHealthService infrastructureHealthService;

    @Inject
    BrokerHealthService brokerHealthService;

    @Inject
    QueueManager queueManager;

    /**
     * Liveness probe endpoint.
     *
     * <p>Returns 200 if the application is alive (not deadlocked, able to process requests).
     * This is a lightweight check that should complete quickly.
     *
     * <p><b>Checks performed:</b>
     * <ul>
     *   <li>Application is running (if we got here, we're alive)</li>
     *   <li>No external dependencies checked</li>
     * </ul>
     *
     * <p><b>Response codes:</b>
     * <ul>
     *   <li>200 OK - Application is alive</li>
     *   <li>503 Service Unavailable - Application is not healthy (should not happen unless severely broken)</li>
     * </ul>
     *
     * @return HTTP 200 if alive, 503 if not alive
     */
    @GET
    @Path("/live")
    @Produces(MediaType.APPLICATION_JSON)
    public Response liveness() {
        try {
            // If we can respond, we're alive
            // Liveness probes should NOT check external dependencies
            // They should only verify the application itself is not deadlocked

            ReadinessStatus status = ReadinessStatus.healthy("ALIVE");

            return Response.ok(status).build();

        } catch (Exception e) {
            LOG.error("Liveness check failed unexpectedly", e);

            ReadinessStatus status = ReadinessStatus.unhealthy(
                "DEAD",
                List.of("Application unable to process requests: " + e.getMessage())
            );

            return Response.status(Response.Status.SERVICE_UNAVAILABLE)
                .entity(status)
                .build();
        }
    }

    /**
     * Readiness probe endpoint.
     *
     * <p>Returns 200 if the application is ready to serve traffic.
     * This includes checking external dependencies like message brokers.
     *
     * <p><b>Checks performed:</b>
     * <ul>
     *   <li>QueueManager is initialized</li>
     *   <li>Message broker (SQS/ActiveMQ) is accessible</li>
     *   <li>Processing pools are operational (not all stalled)</li>
     *   <li>Queue consumers are actively polling (not stuck/dead)</li>
     * </ul>
     *
     * <p><b>Response codes:</b>
     * <ul>
     *   <li>200 OK - Application is ready to serve traffic</li>
     *   <li>503 Service Unavailable - Application is not ready (dependencies down, initialization incomplete)</li>
     * </ul>
     *
     * @return HTTP 200 if ready, 503 if not ready
     */
    @GET
    @Path("/ready")
    @Produces(MediaType.APPLICATION_JSON)
    public Response readiness() {
        List<String> issues = new ArrayList<>();

        try {
            // Check 1: Infrastructure health (QueueManager initialized, pools operational)
            InfrastructureHealth infraHealth = infrastructureHealthService.checkHealth();
            if (!infraHealth.healthy()) {
                issues.addAll(infraHealth.issues());
            }

            // Check 2: Broker connectivity (critical external dependency)
            List<String> brokerIssues = brokerHealthService.checkBrokerConnectivity();
            if (!brokerIssues.isEmpty()) {
                issues.addAll(brokerIssues);
            }

            // Check 3: Consumer health (are consumers actively polling?)
            Map<String, QueueManager.QueueConsumerHealth> consumerHealth = queueManager.getConsumerHealthStatus();
            int totalConsumers = consumerHealth.size();
            int unhealthyConsumers = 0;
            for (QueueManager.QueueConsumerHealth health : consumerHealth.values()) {
                if (!health.isHealthy()) {
                    unhealthyConsumers++;
                    long secondsSinceLastPoll = health.timeSinceLastPollMs() > 0
                        ? health.timeSinceLastPollMs() / 1000 : -1;
                    issues.add(String.format("Consumer for queue %s is unhealthy (last poll %ds ago)",
                        health.queueIdentifier(), secondsSinceLastPoll));
                }
            }

            // If ALL consumers are unhealthy, that's a critical issue
            if (totalConsumers > 0 && unhealthyConsumers == totalConsumers) {
                LOG.errorf("All %d consumers are unhealthy - system cannot process messages", totalConsumers);
            } else if (unhealthyConsumers > 0) {
                LOG.warnf("%d of %d consumers are unhealthy", unhealthyConsumers, totalConsumers);
            }

            // Determine readiness
            boolean ready = issues.isEmpty();

            ReadinessStatus status = ready
                ? ReadinessStatus.healthy("READY")
                : ReadinessStatus.unhealthy("NOT_READY", issues);

            if (ready) {
                LOG.trace("Readiness check passed");
                return Response.ok(status).build();
            } else {
                LOG.warnf("Readiness check failed: %s", issues);
                return Response.status(Response.Status.SERVICE_UNAVAILABLE)
                    .entity(status)
                    .build();
            }

        } catch (Exception e) {
            LOG.error("Readiness check failed with exception", e);

            ReadinessStatus status = ReadinessStatus.unhealthy(
                "ERROR",
                List.of("Readiness check failed: " + e.getMessage())
            );

            return Response.status(Response.Status.SERVICE_UNAVAILABLE)
                .entity(status)
                .build();
        }
    }

    /**
     * Startup probe endpoint (optional, for slow-starting applications).
     *
     * <p>Similar to readiness but with more lenient timeout/failure thresholds.
     * Use this if your application takes a long time to start up.
     *
     * <p>Kubernetes will not run liveness/readiness probes until startup succeeds.
     *
     * @return HTTP 200 if started, 503 if still starting
     */
    @GET
    @Path("/startup")
    @Produces(MediaType.APPLICATION_JSON)
    public Response startup() {
        // For now, startup is the same as readiness
        // Can be enhanced later if we need different startup logic
        return readiness();
    }
}
