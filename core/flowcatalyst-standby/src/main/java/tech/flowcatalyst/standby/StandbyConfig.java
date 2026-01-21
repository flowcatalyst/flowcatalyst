package tech.flowcatalyst.standby;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;

/**
 * Hot standby configuration for distributed primary/standby deployment.
 * When enabled, uses Redis for leader election with automatic failover.
 * Disabled by default - single instance mode requires no Redis.
 */
@ConfigMapping(prefix = "standby")
public interface StandbyConfig {

    /**
     * Enable hot standby mode with Redis-based leader election.
     * If false, system operates as single instance without Redis dependency.
     */
    @WithDefault("false")
    boolean enabled();

    /**
     * Unique instance identifier for this server.
     * Used to identify which instance holds the lock.
     * Defaults to HOSTNAME environment variable or "instance-1".
     */
    @WithDefault("${HOSTNAME:instance-1}")
    String instanceId();

    /**
     * Redis key name for the distributed lock.
     * Each service should use a unique key (e.g., "message-router-primary-lock",
     * "event-processor-primary-lock").
     */
    @WithDefault("standby-primary-lock")
    String lockKey();

    /**
     * Lock TTL in seconds.
     * If lock holder doesn't refresh within this time, lock expires and standby can take over.
     * Recommended: 30 seconds to detect failures quickly while allowing for network jitter.
     *
     * Note: Lock refresh interval is hardcoded to 10 seconds (1/3 of default TTL).
     */
    @WithDefault("30")
    int lockTtlSeconds();
}
