package tech.flowcatalyst.messagerouter.traffic;

import jakarta.enterprise.context.ApplicationScoped;

import java.util.logging.Logger;

/**
 * No-op traffic management strategy.
 *
 * Does nothing - maintains current behavior where both PRIMARY and STANDBY
 * instances remain registered with the load balancer.
 *
 * This is the default strategy when traffic management is disabled or
 * when no specific strategy is needed.
 */
@ApplicationScoped
public class NoOpTrafficStrategy implements TrafficManagementStrategy {

    private static final Logger LOG = Logger.getLogger(NoOpTrafficStrategy.class.getName());

    @Override
    public void registerAsActive() throws TrafficManagementException {
        LOG.fine("NoOp strategy: registerAsActive() - no action taken");
    }

    @Override
    public void deregisterFromActive() throws TrafficManagementException {
        LOG.fine("NoOp strategy: deregisterFromActive() - no action taken");
    }

    @Override
    public boolean isRegistered() {
        // Always return true since we don't manage registration
        return true;
    }

    @Override
    public TrafficStatus getStatus() {
        return new TrafficStatus(
                "noop",
                true,
                "No traffic management - all instances receive traffic",
                "none",
                null
        );
    }
}
