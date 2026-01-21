package tech.flowcatalyst.messagerouter.traffic;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;
import jakarta.enterprise.context.ApplicationScoped;

import java.util.Optional;

/**
 * Configuration for traffic management strategies.
 *
 * Controls how instances register/deregister from load balancers
 * based on their PRIMARY/STANDBY role.
 */
@ConfigMapping(prefix = "traffic-management")
@ApplicationScoped
public interface TrafficManagementConfig {

    /**
     * Enable traffic management integration.
     * If false, no traffic management operations are performed.
     */
    @WithDefault("false")
    boolean enabled();

    /**
     * Traffic management strategy to use.
     * Options: noop, aws-alb
     */
    @WithDefault("noop")
    String strategy();

    // AWS ALB Configuration

    /**
     * AWS ALB target group ARN.
     * Required when strategy is "aws-alb".
     */
    Optional<String> albTargetGroupArn();

    /**
     * Target ID for ALB registration (typically EC2 instance ID or IP address).
     * Required when strategy is "aws-alb".
     */
    Optional<String> albTargetId();

    /**
     * Target port for ALB registration.
     * Defaults to 8080.
     */
    @WithDefault("8080")
    int albTargetPort();

    /**
     * Deregistration delay in seconds (connection draining timeout).
     * Defaults to 300 seconds (5 minutes).
     */
    @WithDefault("300")
    int albDeregistrationDelaySeconds();
}
