package tech.flowcatalyst.messagerouter.traffic;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;
import jakarta.enterprise.context.ApplicationScoped;

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
     * Options: noop
     */
    @WithDefault("noop")
    String strategy();
}
