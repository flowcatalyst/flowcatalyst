package tech.flowcatalyst.messagerouter.traffic;

/**
 * Strategy interface for managing traffic routing to this instance.
 *
 * Different deployment environments can implement this to control
 * whether the load balancer routes traffic to this instance based
 * on its PRIMARY/STANDBY role.
 *
 * Implementations should be:
 * - Idempotent (safe to call multiple times)
 * - Non-blocking (use async operations if needed)
 * - Gracefully degrading (failures should log but not crash)
 */
public interface TrafficManagementStrategy {

    /**
     * Register this instance as active with the load balancer.
     * Called when instance becomes PRIMARY.
     *
     * @throws TrafficManagementException if registration fails
     */
    void registerAsActive() throws TrafficManagementException;

    /**
     * Deregister this instance from the load balancer.
     * Called when instance becomes STANDBY or shuts down.
     *
     * @throws TrafficManagementException if deregistration fails
     */
    void deregisterFromActive() throws TrafficManagementException;

    /**
     * Check if this instance is currently registered with the load balancer.
     *
     * @return true if registered, false otherwise
     */
    boolean isRegistered();

    /**
     * Get the current status for monitoring/debugging.
     *
     * @return status information
     */
    TrafficStatus getStatus();

    /**
     * Exception thrown when traffic management operations fail.
     */
    class TrafficManagementException extends Exception {
        public TrafficManagementException(String message) {
            super(message);
        }

        public TrafficManagementException(String message, Throwable cause) {
            super(message, cause);
        }
    }

    /**
     * Status information for monitoring/debugging.
     */
    class TrafficStatus {
        public final String strategyType;
        public final boolean registered;
        public final String targetInfo;
        public final String lastOperation;
        public final String lastError;

        public TrafficStatus(String strategyType, boolean registered, String targetInfo,
                           String lastOperation, String lastError) {
            this.strategyType = strategyType;
            this.registered = registered;
            this.targetInfo = targetInfo;
            this.lastOperation = lastOperation;
            this.lastError = lastError;
        }
    }
}
