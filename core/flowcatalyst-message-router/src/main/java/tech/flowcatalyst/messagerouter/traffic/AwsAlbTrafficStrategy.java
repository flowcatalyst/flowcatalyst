package tech.flowcatalyst.messagerouter.traffic;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import software.amazon.awssdk.services.elasticloadbalancingv2.ElasticLoadBalancingV2Client;
import software.amazon.awssdk.services.elasticloadbalancingv2.model.*;

import java.util.List;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * AWS ALB target group registration/deregistration strategy.
 *
 * Handles registration/deregistration from an ALB target group
 * for standby mode support. When transitioning to STANDBY, deregisters
 * from the target group and waits for connection draining. When transitioning
 * to PRIMARY, re-registers with the target group.
 *
 * This strategy is idempotent - calling registerAsActive() when already
 * registered or deregisterFromActive() when already deregistered is safe.
 *
 * Matches TypeScript AwsAlbStrategy behavior.
 */
@ApplicationScoped
public class AwsAlbTrafficStrategy implements TrafficManagementStrategy {

    private static final Logger LOG = Logger.getLogger(AwsAlbTrafficStrategy.class.getName());
    private static final long POLL_INTERVAL_MS = 5000; // 5 seconds between health checks

    @Inject
    TrafficManagementConfig config;

    @Inject
    ElasticLoadBalancingV2Client elbClient;

    private volatile boolean registered = false;
    private volatile String lastOperation = "none";
    private volatile String lastError = null;

    @Override
    public void registerAsActive() throws TrafficManagementException {
        String targetGroupArn = config.albTargetGroupArn().orElseThrow(
            () -> new TrafficManagementException("ALB target group ARN not configured"));
        String targetId = config.albTargetId().orElseThrow(
            () -> new TrafficManagementException("ALB target ID not configured"));
        int targetPort = config.albTargetPort();

        LOG.info("Registering target with ALB target group: " + targetGroupArn);
        lastOperation = "register";
        lastError = null;

        try {
            TargetDescription target = TargetDescription.builder()
                .id(targetId)
                .port(targetPort)
                .build();

            RegisterTargetsRequest request = RegisterTargetsRequest.builder()
                .targetGroupArn(targetGroupArn)
                .targets(List.of(target))
                .build();

            elbClient.registerTargets(request);
            registered = true;
            LOG.info("Target registered successfully with ALB target group");

        } catch (ElasticLoadBalancingV2Exception e) {
            lastError = e.getMessage();
            LOG.log(Level.SEVERE, "Failed to register target with ALB: " + e.getMessage(), e);
            throw new TrafficManagementException("Failed to register target with ALB: " + e.getMessage(), e);
        }
    }

    @Override
    public void deregisterFromActive() throws TrafficManagementException {
        String targetGroupArn = config.albTargetGroupArn().orElseThrow(
            () -> new TrafficManagementException("ALB target group ARN not configured"));
        String targetId = config.albTargetId().orElseThrow(
            () -> new TrafficManagementException("ALB target ID not configured"));
        int targetPort = config.albTargetPort();
        int deregistrationDelaySeconds = config.albDeregistrationDelaySeconds();

        LOG.info("Deregistering target from ALB target group: " + targetGroupArn);
        lastOperation = "deregister";
        lastError = null;

        try {
            TargetDescription target = TargetDescription.builder()
                .id(targetId)
                .port(targetPort)
                .build();

            DeregisterTargetsRequest request = DeregisterTargetsRequest.builder()
                .targetGroupArn(targetGroupArn)
                .targets(List.of(target))
                .build();

            elbClient.deregisterTargets(request);
            LOG.info("Target deregistration initiated, waiting for draining to complete");

            // Wait for deregistration to complete (draining)
            waitForDeregistration(targetGroupArn, targetId, targetPort, deregistrationDelaySeconds);
            registered = false;
            LOG.info("Target fully deregistered from ALB target group");

        } catch (ElasticLoadBalancingV2Exception e) {
            lastError = e.getMessage();
            LOG.log(Level.SEVERE, "Failed to deregister target from ALB: " + e.getMessage(), e);
            throw new TrafficManagementException("Failed to deregister target from ALB: " + e.getMessage(), e);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            lastError = "Interrupted during deregistration wait";
            throw new TrafficManagementException("Interrupted while waiting for deregistration", e);
        }
    }

    /**
     * Wait for target to be fully deregistered (connection draining complete).
     * Polls target health until state is 'unused' or timeout expires.
     */
    private void waitForDeregistration(String targetGroupArn, String targetId, int targetPort,
                                       int maxWaitSeconds) throws InterruptedException {
        long maxWaitMs = maxWaitSeconds * 1000L;
        long startTime = System.currentTimeMillis();

        TargetDescription targetDesc = TargetDescription.builder()
            .id(targetId)
            .port(targetPort)
            .build();

        DescribeTargetHealthRequest healthRequest = DescribeTargetHealthRequest.builder()
            .targetGroupArn(targetGroupArn)
            .targets(List.of(targetDesc))
            .build();

        while (true) {
            long elapsed = System.currentTimeMillis() - startTime;
            if (elapsed > maxWaitMs) {
                LOG.warning("Deregistration wait timed out after " + elapsed + "ms, proceeding anyway");
                return;
            }

            try {
                DescribeTargetHealthResponse response = elbClient.describeTargetHealth(healthRequest);
                List<TargetHealthDescription> healthDescriptions = response.targetHealthDescriptions();

                if (healthDescriptions.isEmpty()) {
                    // Target no longer in target group
                    LOG.fine("Target no longer in target group, deregistration complete");
                    return;
                }

                TargetHealthDescription healthDesc = healthDescriptions.get(0);
                TargetHealthStateEnum state = healthDesc.targetHealth().state();

                LOG.fine("Target health state during deregistration: " + state + " (elapsed: " + elapsed + "ms)");

                // Target is fully deregistered when state is 'unused' or not found
                if (state == TargetHealthStateEnum.UNUSED) {
                    LOG.info("Target deregistration complete (state: unused)");
                    return;
                }

                // Still draining or in other state, wait and check again
                if (state != TargetHealthStateEnum.DRAINING) {
                    LOG.info("Target in unexpected state during deregistration: " + state);
                }

            } catch (ElasticLoadBalancingV2Exception e) {
                // If target not found, deregistration is complete
                if (e.awsErrorDetails() != null &&
                    "InvalidTarget".equals(e.awsErrorDetails().errorCode())) {
                    LOG.info("Target not found in target group, deregistration complete");
                    return;
                }
                LOG.warning("Error checking target health during deregistration: " + e.getMessage());
            }

            Thread.sleep(POLL_INTERVAL_MS);
        }
    }

    @Override
    public boolean isRegistered() {
        return registered;
    }

    @Override
    public TrafficStatus getStatus() {
        String targetInfo = config.albTargetGroupArn()
            .map(arn -> "arn=" + arn + ", id=" + config.albTargetId().orElse("not-configured") +
                       ", port=" + config.albTargetPort())
            .orElse("not configured");

        return new TrafficStatus(
            "aws-alb",
            registered,
            targetInfo,
            lastOperation,
            lastError
        );
    }
}
