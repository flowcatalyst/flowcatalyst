package tech.flowcatalyst.platform.config;

import io.quarkus.runtime.annotations.StaticInitSafe;
import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;
import io.smallrye.config.WithName;

/**
 * Configuration for platform feature flags.
 *
 * These settings control which features are available in the platform UI.
 * Useful for businesses that don't need certain capabilities.
 *
 * Example configuration:
 * <pre>
 * flowcatalyst.features.messaging-enabled=false
 * </pre>
 */
@StaticInitSafe
@ConfigMapping(prefix = "flowcatalyst.features")
public interface PlatformFeaturesConfig {

    /**
     * Whether the messaging features are enabled.
     * When false, hides Event Types, Subscriptions, Dispatch Pools, and Dispatch Jobs
     * from the platform UI navigation.
     */
    @WithName("messaging-enabled")
    @WithDefault("true")
    boolean messagingEnabled();
}
